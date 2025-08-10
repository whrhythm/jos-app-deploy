package routes

import (
	"context"
	pb "jos-deployment/api/v1alpha1/pb_routes"
)

type RoutesManageService struct {
	pb.UnimplementedAPISIXGatewayServiceServer
}

func (s *RoutesManageService) ListRoutes(ctx context.Context, req *pb.ListRoutesRequest) (*pb.ListRoutesResponse, error) {
	return &pb.ListRoutesResponse{}, nil
}
