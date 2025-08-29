package routes

import (
	"context"
	pb "jos-deployment/api/v1alpha1/pb_routes"
	"jos-deployment/pkg/gateway"
	"jos-deployment/pkg/logger"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func (s *RoutesManageService) CreateApisixRoute(ctx context.Context, req *pb.CreateApisixRouteRequest) (*pb.CreateApisixRouteResponse, error) {
	logger.L().Info("CreateApisixRoute called")

	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, status.Errorf(status.Code(err), "failed to create k8s config: %v", err)
		}
	}

	gw := gateway.NewGateway()
	var httpRoutes []gateway.HTTPRoute
	for _, r := range req.Http {
		var httpBackends []gateway.Backend
		for _, m := range r.Backends {
			httpBackend := gateway.Backend{
				ServiceName: m.ServiceName,
				ServicePort: int32(m.ServicePort),
			}
			httpBackends = append(httpBackends, httpBackend)
		}
		httpRoute := gateway.HTTPRoute{
			Hosts:    r.Hosts,
			Paths:    r.Paths,
			Backends: httpBackends,
		}
		httpRoutes = append(httpRoutes, httpRoute)
	}
	logger.L().Info("HTTP Routes: ", zap.Any("httpRoutes", httpRoutes))

	var streamRoutes []gateway.StreamRoute
	for _, r := range req.Stream {
		streamRoute := gateway.StreamRoute{
			IngressPort: int32(r.IngressPort),
			Backend: gateway.Backend{
				ServiceName: r.Backend.ServiceName,
				ServicePort: int32(r.Backend.ServicePort),
			},
		}
		streamRoutes = append(streamRoutes, streamRoute)
	}
	logger.L().Info("Stream Route: ", zap.Any("streamRoute", streamRoutes))

	err = gw.CreateOrUpdateRoute(config, req.ArName, req.Namespace,
		gateway.ConvertHTTPRoutes(httpRoutes),
		gateway.ConvertStreamRoutes(streamRoutes))
	if err != nil {
		return nil, status.Errorf(status.Code(err), "failed to create or update route: %v", err)
	}

	return &pb.CreateApisixRouteResponse{}, nil
}

func (a *RoutesManageService) DeleteApisixRoute(ctx context.Context, req *pb.DeleteApisixRouteRequest) (*pb.DeleteApisixRouteResponse, error) {

	return &pb.DeleteApisixRouteResponse{}, nil
}
