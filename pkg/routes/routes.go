package routes

import (
	"context"
	pb "jos-deployment/api/v1alpha1/pb_routes"
	"os"
	"path/filepath"

	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type RoutesManageService struct {
	pb.UnimplementedAPISIXGatewayServiceServer
}

func (s *RoutesManageService) ListRoutes(ctx context.Context, req *pb.ListRoutesRequest) (*pb.ListRoutesResponse, error) {
	namespace := req.GetNamespace()
	// releaseName := req.GetReleaseName()

	errRsp := &pb.ListRoutesResponse{
		Code:    2,
		Success: false,
		Message: "",
		Rules:   nil,
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
		Rules:   routeRules,
	}, nil
}

func (s *RoutesManageService) ListCerts(ctx context.Context, req *pb.ListTLSRequest) (*pb.ListTLSResponse, error) {
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
			tlsData := &pb.TLSData{
				Name:    cert.Name,
				Source:  "kubernetes",
				DnsName: string(cert.Data["tls.crt"]), // Placeholder, parse actual DNS names from the certificate
				Expired: "unknown",                    // Placeholder, parse actual expiration date from the certificate
			}
			tlsDataList = append(tlsDataList, tlsData)
		}
	}

	return &pb.ListTLSResponse{
		Data: tlsDataList,
	}, nil
}
