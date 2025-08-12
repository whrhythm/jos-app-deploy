package routes

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	pb "jos-deployment/api/v1alpha1/pb_routes"
	"jos-deployment/pkg/logger"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type RoutesManageService struct {
	pb.UnimplementedAPISIXGatewayServiceServer
}

func (s *RoutesManageService) ListRoutes(ctx context.Context, req *pb.ListRoutesRequest) (*pb.ListRoutesResponse, error) {
	logger.L().Info("ListRoutes called", zap.String("namespace", req.GetNamespace()))
	namespace := req.GetNamespace()
	// releaseName := req.GetReleaseName()
	errRsp := &pb.ListRoutesResponse{
		Code:    2,
		Success: false,
		Message: "",
		Data:    nil,
	}

	// Initialize Kubernetes clientset
	config, err := rest.InClusterConfig()
	if err != nil {
		// 尝试使用本地 kubeconfig
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

	// 通过标签获取 ingress 列表
	ingresses, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to list ingresses: %v", err)
	}

	var routeRules []*pb.RouteRule
	for _, ingress := range ingresses.Items {
		for _, rule := range ingress.Spec.Rules {
			routeRule := &pb.RouteRule{
				Host: rule.Host,
			}
			for _, path := range rule.HTTP.Paths {
				backend := &pb.RouteBackend{
					Name: path.Backend.Service.Name,
					Port: path.Backend.Service.Port.Number,
					Path: path.Path,
				}
				routeRule.Paths = append(routeRule.Paths, backend)
			}
			routeRules = append(routeRules, routeRule)
		}
	}
	return &pb.ListRoutesResponse{
		Code:    0,
		Success: true,
		Data: &pb.ListRoutesData{
			Rules: routeRules,
		},
	}, nil
}

func parseCertInfo(certData []byte) (dnsNames []string, notAfter string) {
	block, _ := pem.Decode(certData)
	if block == nil {
		logger.L().Error("Failed to parse certificate PEM block")
		return nil, ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		logger.L().Error("Failed to parse certificate")
		return nil, ""
	}
	return cert.DNSNames, cert.NotAfter.Format("2006-01-02 15:04:05")
}

func (s *RoutesManageService) ListCerts(ctx context.Context, req *pb.ListTLSRequest) (*pb.ListTLSResponse, error) {
	logger.L().Info("ListCerts called", zap.String("namespace", req.GetNamespace()))
	namespace := req.GetNamespace()
	// releaseName := req.GetReleaseName()

	errRsp := &pb.ListTLSResponse{}

	// Initialize Kubernetes clientset
	config, err := rest.InClusterConfig()
	if err != nil {
		// 尝试使用本地 kubeconfig
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

	certificates, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return errRsp, status.Errorf(status.Code(err), "failed to list certificates: %v", err)
	}

	var tlsDataList []*pb.TLSData
	for _, cert := range certificates.Items {
		if cert.Type == "kubernetes.io/tls" {
			dnsName, expired := parseCertInfo(cert.Data["tls.crt"])
			tlsData := &pb.TLSData{
				Name:    cert.Name,
				Source:  "kubernetes",
				DnsName: "",      // Placeholder, will be set below
				Expired: expired, // Placeholder, parse actual expiration date from the certificate
			}
			if len(dnsName) > 0 {
				tlsData.DnsName = dnsName[0]
			}
			tlsDataList = append(tlsDataList, tlsData)
		}
	}

	return &pb.ListTLSResponse{
		Code:    0,
		Success: true,
		Message: "Successfully retrieved TLS certificates",
		Data:    tlsDataList,
	}, nil
}

func (s *RoutesManageService) CreateRoute(ctx context.Context, req *pb.CreateRouteRequest) (*pb.CreateRouteResponse, error) {
	logger.L().Info("CreateRoute called", zap.String("namespace", req.GetNamespace()), zap.String("ingressName", req.GetIngName()))
	namespace := req.GetNamespace()
	// 默认更新是false
	update := false
	errRsp := &pb.CreateRouteResponse{
		Code:    2,
		Success: false,
		Message: "",
	}

	// Initialize Kubernetes clientset
	config, err := rest.InClusterConfig()
	if err != nil {
		// 尝试使用本地 kubeconfig
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

	var ingressRules []networkingv1.IngressRule
	routeName := req.GetIngName()
	for _, rule := range req.GetRules() {
		var paths []networkingv1.HTTPIngressPath
		if routeName == "" {
			return errRsp, status.Errorf(400, "ingress name cannot be empty")
		}
		// 检查 Ingress 是否已存在
		_, err = clientset.NetworkingV1().Ingresses(namespace).Get(ctx, routeName, metav1.GetOptions{})
		if err == nil {
			// 已经存在，更新Ingress
			update = true
		}
		for _, path := range rule.GetPaths() {
			paths = append(paths, networkingv1.HTTPIngressPath{
				Path: path.GetPath(),
				PathType: func() *networkingv1.PathType {
					pt := networkingv1.PathTypePrefix
					return &pt
				}(),
				Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: path.GetName(),
						Port: networkingv1.ServiceBackendPort{
							Number: path.GetPort(),
						},
					},
				},
			})
		}
		ingressRules = append(ingressRules, networkingv1.IngressRule{
			Host: rule.GetHost(),
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: paths,
				},
			},
		})
	}

	// 创建 Ingress 对象
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: ingressRules,
		},
	}

	// 创建 Ingress
	if update {
		_, err = clientset.NetworkingV1().Ingresses(namespace).Update(ctx, ingress, metav1.UpdateOptions{})
		if err != nil {
			logger.L().Error("Failed to update ingress", zap.Error(err))
			return errRsp, status.Errorf(status.Code(err), "failed to update ingress: %v", err)
		}
	} else {
		_, err = clientset.NetworkingV1().Ingresses(namespace).Create(ctx, ingress, metav1.CreateOptions{})
		if err != nil {
			logger.L().Error("Failed to create ingress", zap.Error(err))
			return errRsp, status.Errorf(status.Code(err), "failed to create ingress: %v", err)
		}
	}
	return &pb.CreateRouteResponse{
		Code:    0,
		Success: true,
		Message: "Route created successfully",
	}, nil
}

func (s *RoutesManageService) GetServiceList(ctx context.Context, req *pb.GetServiceListRequest) (*pb.GetServiceListResponse, error) {
	logger.L().Info("GetServiceList called", zap.String("namespace", req.GetNamespace()))
	namespace := req.GetNamespace()
	// Initialize Kubernetes clientset
	config, err := rest.InClusterConfig()
	if err != nil {
		// 尝试使用本地 kubeconfig
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, status.Errorf(status.Code(err), "failed to create k8s config: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, status.Errorf(status.Code(err), "failed to create Kubernetes clientset: %v", err)
	}

	// 获取指定命名空间下的所有 Service
	services, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, status.Errorf(status.Code(err), "failed to list services: %v", err)
	}

	var serviceList []*pb.GetServiceData
	for _, svc := range services.Items {
		var ports []int32
		for _, p := range svc.Spec.Ports {
			ports = append(ports, p.Port)
		}
		serviceData := &pb.GetServiceData{
			Name:  svc.Name,
			Ports: ports,
		}
		serviceList = append(serviceList, serviceData)
	}

	return &pb.GetServiceListResponse{
		Code:    0,
		Success: true,
		Message: "Successfully retrieved service list",
		Data:    serviceList,
	}, nil
}

func (s *RoutesManageService) DeleteRoute(ctx context.Context, req *pb.DeleteRouteRequest) (*pb.DeleteRouteResponse, error) {
	logger.L().Info("DeleteRoute called", zap.String("namespace", req.GetNamespace()), zap.String("routeName", req.GetRouteName()))
	namespace := req.GetNamespace()
	routeName := req.GetRouteName()

	// Initialize Kubernetes clientset
	config, err := rest.InClusterConfig()
	if err != nil {
		// 尝试使用本地 kubeconfig
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, status.Errorf(status.Code(err), "failed to create k8s config: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, status.Errorf(status.Code(err), "failed to create Kubernetes clientset: %v", err)
	}

	err = clientset.NetworkingV1().Ingresses(namespace).Delete(ctx, routeName, metav1.DeleteOptions{})
	if err != nil {
		return nil, status.Errorf(status.Code(err), "failed to delete ingress: %v", err)
	}

	return &pb.DeleteRouteResponse{
		Success: true,
		Message: "Route deleted successfully",
	}, nil
}

func (s *RoutesManageService) CreateUpdateTLS(ctx context.Context, req *pb.CreateUPdateTLSRequest) (*pb.CreateUPdateTLSResponse, error) {
	logger.L().Info("CreateUpdateTLS called", zap.String("namespace", req.GetNamespace()))

	namespace := req.GetNamespace()
	certName := req.GetName()
	certData := req.GetCrt()
	keyData := req.GetKey()

	// Initialize Kubernetes clientset
	config, err := rest.InClusterConfig()
	if err != nil {
		// 尝试使用本地 kubeconfig
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, status.Errorf(status.Code(err), "failed to create k8s config: %v", err)
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, status.Errorf(status.Code(err), "failed to create Kubernetes clientset: %v", err)
	}
	// 检查 Secret 是否已存在
	_, err = clientset.CoreV1().Secrets(namespace).Get(ctx, certName, metav1.GetOptions{})
	if err == nil {
		return nil, status.Errorf(409, "TLS secret with name %s already exists", certName)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte(certData),
			"tls.key": []byte(keyData),
		},
		Type: "kubernetes.io/tls",
	}
	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		logger.L().Error("Failed to create TLS secret", zap.Error(err))
		return nil, status.Errorf(status.Code(err), "failed to create TLS secret: %v", err)
	}

	return &pb.CreateUPdateTLSResponse{
		Code:    0,
		Message: "TLS secret created successfully",
	}, nil
}
