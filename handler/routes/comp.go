package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	pb "jos-deployment/api/v1alpha1/pb_routes"
	"jos-deployment/pkg/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	defaultHarborUserName = "admin"
	defaultHarborPassword = "P@88w0rd"
	defaultHarborAddress  = "harbor-core.harbor.svc.cluster.local"
)

func (s *RoutesManageService) GetDeployListFromPod(ctx context.Context, req *pb.GetDeployListFromPodRequest) (*pb.GetDeployListFromPodResponse, error) {
	logger.L().Info("GetDeployListFromPod called", zap.String("namespace", req.GetNamespace()), zap.String("pod", req.GetName()))
	namespace := req.GetNamespace()
	podName := req.GetName()

	errRsp := &pb.GetDeployListFromPodResponse{}

	// Initialize Kubernetes clientset (in-cluster or kubeconfig)
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return errRsp, status.Errorf(status.Code(err), "failed to create k8s config: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to create Kubernetes clientset: %v", err)
	}

	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to get pod: %v", err)
	}

	// 提取Controller信息
	var data []*pb.GetDeployListFromPodResponseData
	for _, owner := range pod.OwnerReferences {
		serviceName := ""
		if owner.Controller != nil && *owner.Controller {
			logger.L().Info("Pod owner", zap.String("name", owner.Name),
				zap.String("kind", owner.Kind),
				zap.String("apiVersion", owner.APIVersion))
			// 如果owner.Kind是ReplicaSet，则查找对应的Deployment
			if owner.Kind == "ReplicaSet" {
				rs, err := clientset.AppsV1().ReplicaSets(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
				if err != nil {
					logger.L().Error("failed to get ReplicaSet", zap.String("name", owner.Name), zap.Error(err))
					return &pb.GetDeployListFromPodResponse{
						Code:    1,
						Success: false,
						Message: fmt.Sprintf("failed to get ReplicaSet: %v", err),
					}, nil
				}
				for _, rsOwner := range rs.OwnerReferences {
					if rsOwner.Controller != nil && *rsOwner.Controller && rsOwner.Kind == "Deployment" {
						owner.Name = rsOwner.Name
						owner.Kind = rsOwner.Kind
						break
					}
				}
			}
			serviceNames := findServicesForPod(ctx, clientset, namespace, pod)
			if len(serviceNames) > 0 {
				serviceName = serviceNames[0]
			}
			data = append(data, &pb.GetDeployListFromPodResponseData{
				Namespace:   namespace,
				DeployName:  owner.Name,
				Kind:        owner.Kind,
				ServiceName: serviceName,
			})
		} else {
			logger.L().Info("Pod owner is not a controller", zap.String("name", owner.Name),
				zap.String("kind", owner.Kind),
				zap.String("apiVersion", owner.APIVersion))
			return &pb.GetDeployListFromPodResponse{
				Code:    1,
				Success: false,
				Message: "Pod owner is not a controller",
			}, nil
		}
	}

	return &pb.GetDeployListFromPodResponse{
		Code:    0,
		Success: true,
		Message: "success",
		Data:    data,
	}, nil
}

func findServicesForPod(ctx context.Context, clientset *kubernetes.Clientset, namespace string, pod *corev1.Pod) []string {
	// Find services that select this pod
	services, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.L().Error("failed to list services", zap.Error(err))
		return nil
	}

	var serviceNames []string
	for _, svc := range services.Items {
		if len(svc.Spec.Selector) == 0 {
			continue
		}
		matches := true
		logger.L().Info("Service Selector", zap.Any("selector", svc.Spec.Selector))
		for key, val := range svc.Spec.Selector {
			if podVal, ok := pod.Labels[key]; !ok || podVal != val {
				matches = false
				break
			}
		}
		if matches {
			serviceNames = append(serviceNames, svc.Name)
		}
	}

	return serviceNames
}

func (s *RoutesManageService) GetDefaultHarborProject(ctx context.Context, req *pb.GetDefaultHarborProjectRequest) (*pb.GetDefaultHarborProjectResponse, error) {
	logger.L().Info("GetDefaultHarborProject called")
	// 访问默认Harbor项目，获取project list
	errRsp := &pb.GetDefaultHarborProjectResponse{}

	// build harbor API url (try v2.0 projects endpoint first)
	harborBase := defaultHarborAddress
	if !strings.HasPrefix(harborBase, "http") {
		harborBase = "http://" + harborBase
	}
	// 工程数不能超过100
	apiUrl := harborBase + "/api/v2.0/projects?page_size=100"

	// simple HTTP request with basic auth
	type harborProj struct {
		Name string `json:"name"`
	}

	client := &http.Client{}
	reqHttp, err := http.NewRequestWithContext(ctx, "GET", apiUrl, nil)
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to create request: %v", err)
	}
	reqHttp.SetBasicAuth(defaultHarborUserName, defaultHarborPassword)
	reqHttp.Header.Set("Accept", "application/json")

	resp, err := client.Do(reqHttp)
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to call harbor API: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errRsp, status.Errorf(codes.Internal, "harbor API returned status %d", resp.StatusCode)
	}

	var projects []harborProj
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&projects); err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to parse harbor response: %v", err)
	}

	var names []string
	for _, p := range projects {
		names = append(names, p.Name)
	}

	return &pb.GetDefaultHarborProjectResponse{
		Code:    0,
		Success: true,
		Message: "success",
		Data:    names,
	}, nil
}

func (s *RoutesManageService) GetHarborProjectImages(ctx context.Context, req *pb.GetHarborProjectImagesRequest) (*pb.GetHarborProjectImagesResponse, error) {
	logger.L().Info("GetHarborProjectImages called", zap.String("project", req.GetProjectName()))

	// 访问默认Harbor项目，获取project list
	errRsp := &pb.GetHarborProjectImagesResponse{}
	if req.GetProjectName() == "" {
		return errRsp, status.Errorf(codes.InvalidArgument, "missing project name")
	}

	// build harbor API url (try v2.0 projects endpoint first)
	harborBase := defaultHarborAddress
	if !strings.HasPrefix(harborBase, "http") {
		harborBase = "http://" + harborBase
	}
	apiUrl := harborBase + "/api/v2.0/projects/" + req.GetProjectName() + "/repositories"
	// simple HTTP request with basic auth
	type harborRepo struct {
		Name string `json:"name"`
	}

	client := &http.Client{}
	reqHttp, err := http.NewRequestWithContext(ctx, "GET", apiUrl, nil)
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to create request: %v", err)
	}
	reqHttp.SetBasicAuth(defaultHarborUserName, defaultHarborPassword)
	reqHttp.Header.Set("Accept", "application/json")

	resp, err := client.Do(reqHttp)
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to call harbor API: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errRsp, status.Errorf(codes.Internal, "harbor API returned status %d", resp.StatusCode)
	}

	// decode into a generic structure so we can handle different tag shapes
	var repos []harborRepo
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&repos); err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to parse harbor response: %v", err)
	}

	var images []*pb.GetHarborImage
	for _, r := range repos {
		var tags []string
		tags = append(tags, getImageTags(req.GetProjectName(), r.Name)...)
		images = append(images, &pb.GetHarborImage{
			Repository: r.Name,
			Tags:       tags,
		})
	}

	return &pb.GetHarborProjectImagesResponse{
		Code:    0,
		Success: true,
		Message: "success",
		Data:    images,
	}, nil
}

func getImageTags(projectName, repoName string) []string {
	harborBase := defaultHarborAddress
	if !strings.HasPrefix(harborBase, "http") {
		harborBase = "http://" + harborBase
	}
	repoName = strings.TrimPrefix(repoName, projectName+"/")
	apiUrl := harborBase + "/api/v2.0/projects/" + projectName + "/repositories/" + repoName + "/artifacts"

	type harborTag struct {
		Name string `json:"name"`
	}

	client := &http.Client{}
	reqHttp, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		logger.L().Error("failed to create request", zap.Error(err))
		return nil
	}
	reqHttp.SetBasicAuth(defaultHarborUserName, defaultHarborPassword)
	reqHttp.Header.Set("Accept", "application/json")

	resp, err := client.Do(reqHttp)
	if err != nil {
		logger.L().Error("failed to call harbor API", zap.Error(err))
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger.L().Error("harbor API returned error status", zap.Int("status", resp.StatusCode))
		return nil
	}

	var artifacts []struct {
		Tags []harborTag `json:"tags"`
	}

	dec := json.NewDecoder(resp.Body)
	logger.L().Info("dec", zap.Any("decoder", dec))
	if err := dec.Decode(&artifacts); err != nil {
		logger.L().Error("failed to parse harbor response", zap.Error(err))
		return nil
	}
	logger.L().Info("artifacts", zap.Any("artifacts", artifacts))
	var tags []string
	for _, art := range artifacts {
		for _, tag := range art.Tags {
			tags = append(tags, tag.Name)
		}
	}
	return tags
}

func (s *RoutesManageService) CreateComponment(ctx context.Context, req *pb.CreateComponmentRequest) (*pb.CreateComponmentResponse, error) {
	logger.L().Info("CreateComponment called")
	name := req.GetName()
	namespace := req.GetDeployInfo().Namespace
	compName := req.GetDeployInfo().DeployName
	image := req.GetImageFuleName()
	controlledBy := req.GetDeployInfo().Kind
	service := req.GetDeployInfo().ServiceName

	logger.L().Info("CreateComponment params", zap.String("name", name),
		zap.String("namespace", namespace),
		zap.String("compName", compName),
		zap.String("image", image),
		zap.String("controlledBy", controlledBy),
		zap.String("service", service),
	)

	if namespace == "" || compName == "" || image == "" || controlledBy == "" || service == "" {
		return &pb.CreateComponmentResponse{Code: 1, Message: "missing required fields", Success: false}, nil
	}

	errRsp := &pb.CreateComponmentResponse{Code: 1, Message: "failed", Success: false}

	// Initialize Kubernetes clientset (in-cluster or kubeconfig)
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return errRsp, status.Errorf(status.Code(err), "failed to create k8s config: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to create Kubernetes clientset: %v", err)
	}

	// Check if namespace exists
	_, err = clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to get namespace: %v", err)
	}

	// Create deployment and service
	switch controlledBy {
	case "Deployment":
		err = createDeployment(ctx, clientset, namespace, name, compName, image)
	case "StatefulSet":
		err = createSts(ctx, clientset, namespace, name, compName, image)
	default:
		return &pb.CreateComponmentResponse{Code: 1, Message: "unsupported controlledBy kind", Success: false}, nil
	}
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to create deployment: %v", err)
	}

	// 创建 Service
	err = createService(ctx, clientset, namespace, name, compName, service)
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to create service: %v", err)
	}

	return &pb.CreateComponmentResponse{Code: 0, Message: "success", Success: true}, nil
}

func createDeployment(ctx context.Context, clientset *kubernetes.Clientset, namespace, name, compName, image string) error {
	logger.L().Info("CreateDeployment called", zap.String("namespace", namespace), zap.String("name", name), zap.String("compName", compName), zap.String("image", image))
	// 更新名字为compName, kind 为controlledBy的资源
	dep, err := clientset.AppsV1().Deployments(namespace).Get(ctx, compName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// deep copy and adjust
	newDep := dep.DeepCopy()
	// reset metadata that should not be carried over
	newDepName := fmt.Sprintf("%s-%s", compName, name)
	newDep.ObjectMeta = metav1.ObjectMeta{
		Name:      newDepName,
		Namespace: namespace,
		Labels:    map[string]string{},
	}
	// preserve some labels from original template
	for k, v := range dep.Spec.Template.ObjectMeta.Labels {
		newDep.ObjectMeta.Labels[k] = v
	}
	// add our component label
	newDep.ObjectMeta.Labels["joiningos.com/mode"] = "customize"

	// adjust selector: ensure matchLabels exists and includes our component label
	if newDep.Spec.Selector == nil {
		newDep.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{}}
	}
	if newDep.Spec.Selector.MatchLabels == nil {
		newDep.Spec.Selector.MatchLabels = map[string]string{}
	}
	// newDep.Spec.Selector.MatchLabels["joiningos.com/componment"] = compName

	// adjust pod template labels
	if newDep.Spec.Template.ObjectMeta.Labels == nil {
		newDep.Spec.Template.ObjectMeta.Labels = map[string]string{}
	}
	newDep.Spec.Template.ObjectMeta.Labels["joiningos.com/componment"] = newDepName
	newDep.Spec.Template.ObjectMeta.Labels["joiningos.com/mode"] = "customize"

	// update container images (set for all containers)
	// 目前支持一个容器
	// TODO
	for i := range newDep.Spec.Template.Spec.Containers {
		newDep.Spec.Template.Spec.Containers[i].Image = image
	}

	// clear status
	newDep.Status = appsv1.DeploymentStatus{}

	// create the new deployment
	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, newDep, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func createSts(ctx context.Context, clientset *kubernetes.Clientset, namespace, name, compName, image string) error {
	logger.L().Info("CreateSts called", zap.String("namespace", namespace), zap.String("name", name), zap.String("compName", compName), zap.String("image", image))
	sts, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, compName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// deep copy and adjust
	newSts := sts.DeepCopy()
	newStsName := fmt.Sprintf("%s-%s", compName, name)
	// reset metadata that should not be carried over
	newSts.ObjectMeta = metav1.ObjectMeta{
		Name:      newStsName,
		Namespace: namespace,
		Labels:    map[string]string{},
	}
	// preserve some labels from original template
	for k, v := range sts.Spec.Template.ObjectMeta.Labels {
		newSts.ObjectMeta.Labels[k] = v
	}
	// add our component label
	newSts.ObjectMeta.Labels["joiningos.com/mode"] = "customize"

	// adjust selector: ensure matchLabels exists and includes our component label
	if newSts.Spec.Selector == nil {
		newSts.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{}}
	}
	if newSts.Spec.Selector.MatchLabels == nil {
		newSts.Spec.Selector.MatchLabels = map[string]string{}
	}
	// newSts.Spec.Selector.MatchLabels["joiningos.com/componment"] = compName

	// adjust pod template labels
	if newSts.Spec.Template.ObjectMeta.Labels == nil {
		newSts.Spec.Template.ObjectMeta.Labels = map[string]string{}
	}
	newSts.Spec.Template.ObjectMeta.Labels["joiningos.com/componment"] = newStsName
	newSts.Spec.Template.ObjectMeta.Labels["joiningos.com/mode"] = "customize"

	// update container images (set for all containers)
	// 目前支持一个容器
	// TODO
	for i := range newSts.Spec.Template.Spec.Containers {
		newSts.Spec.Template.Spec.Containers[i].Image = image
	}

	// clear status
	newSts.Status = appsv1.StatefulSetStatus{}

	// create the new deployment
	_, err = clientset.AppsV1().StatefulSets(namespace).Create(ctx, newSts, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func createService(ctx context.Context, clientset *kubernetes.Clientset, namespace, name, compName, service string) error {
	svc, err := clientset.CoreV1().Services(namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return err
	}
	newSvc := svc.DeepCopy()
	newSvc.ObjectMeta = metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-%s", compName, name),
		Namespace: namespace,
		Labels:    map[string]string{},
	}
	// preserve some labels from original service
	for k, v := range svc.ObjectMeta.Labels {
		newSvc.ObjectMeta.Labels[k] = v
	}
	// add our component label
	newSvc.ObjectMeta.Labels["joiningos.com/mode"] = "customize"
	// adjust selector to match our component label
	if newSvc.Spec.Selector == nil {
		newSvc.Spec.Selector = map[string]string{}
	}
	newSvc.Spec.Selector["joiningos.com/componment"] = fmt.Sprintf("%s-%s", compName, name)
	// clear clusterIP to let k8s assign a new one
	newSvc.Spec.ClusterIP = ""
	newSvc.Spec.ClusterIPs = nil
	// clear status
	newSvc.Status = corev1.ServiceStatus{}
	_, err = clientset.CoreV1().Services(namespace).Create(ctx, newSvc, metav1.CreateOptions{})
	return err
}

func (s *RoutesManageService) DeleteComponment(ctx context.Context, req *pb.DeleteComponmentRequest) (*pb.DeleteComponmentResponse, error) {
	logger.L().Info("DeleteComponment called", zap.String("namespace", req.GetNamespace()), zap.String("podName", req.GetPodName()))
	deployInfo, err := s.GetDeployListFromPod(ctx, &pb.GetDeployListFromPodRequest{
		Namespace: req.GetNamespace(),
		Name:      req.GetPodName(),
	})
	if err != nil {
		return nil, status.Errorf(status.Code(err), "failed to get deploy info from pod: %v", err)
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, status.Errorf(status.Code(err), "failed to create k8s config: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, status.Errorf(status.Code(err), "failed to create k8s clientset: %v", err)
	}
	switch deployInfo.Data[0].Kind {
	case "Deployment":
		// 删除 Deployment
		err = clientset.AppsV1().Deployments(req.GetNamespace()).Delete(ctx, deployInfo.Data[0].DeployName, metav1.DeleteOptions{
			PropagationPolicy: func() *metav1.DeletionPropagation {
				p := metav1.DeletePropagationForeground
				return &p
			}(),
		})
		if err != nil {
			return nil, status.Errorf(status.Code(err), "failed to delete deployment: %v", err)
		}
	case "StatefulSet":
		// 删除 StatefulSet
		err = clientset.AppsV1().StatefulSets(req.GetNamespace()).Delete(ctx, deployInfo.Data[0].DeployName, metav1.DeleteOptions{
			PropagationPolicy: func() *metav1.DeletionPropagation {
				p := metav1.DeletePropagationForeground
				return &p
			}(),
		})
		if err != nil {
			return nil, status.Errorf(status.Code(err), "failed to delete statefulset: %v", err)
		}
	default:
		return &pb.DeleteComponmentResponse{
			Code:    1,
			Success: false,
			Message: "unsupported kind",
		}, nil
	}

	// TODO 没有删除Service

	return &pb.DeleteComponmentResponse{
		Code:    0,
		Success: true,
		Message: "success",
	}, nil
}
