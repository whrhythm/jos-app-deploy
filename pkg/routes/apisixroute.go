package routes

import (
	"context"
	pb "jos-deployment/api/v1alpha1/pb_routes"
	"jos-deployment/pkg/logger"
)

// apiVersion: apisix.apache.org/v2
// kind: ApisixRoute
// metadata:
//
//	name: weighted-routing-example
//	namespace: default
//
// spec:
//
//	http:
//	- name: rule1
//	  match:
//	    hosts:
//	    - example.com
//	    paths:
//	    - /api/v1/*
//	  backends:
//	  # 第一个服务 - 权重 70%
//	  - serviceName: service-a
//	    servicePort: 80
//	    weight: 70
//	  # 第二个服务 - 权重 30%
//	  - serviceName: service-b
//	    servicePort: 80
//	    weight: 30
//	  # 可选的插件配置
func (s *RoutesManageService) CreateApisixRoute(ctx context.Context, req *pb.CreateApisixRouteRequest) (*pb.CreateApisixRouteResponse, error) {
	logger.L().Info("CreateApisixRoute called")

	return &pb.CreateApisixRouteResponse{}, nil
}

func (a *RoutesManageService) DeleteApisixRoute(ctx context.Context, req *pb.DeleteApisixRouteRequest) (*pb.DeleteApisixRouteResponse, error) {

	return &pb.DeleteApisixRouteResponse{}, nil
}
