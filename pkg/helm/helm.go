package helm

import (
	"context"
	"jos-deployment/pkg/logger"

	pb "jos-deployment/api/v1alpha1/pb"

	"go.uber.org/zap"
)

type HelmManagerServer struct {
	pb.UnimplementedHelmManagerServiceServer
}

type RepositoryConfig struct {
	Repositories []RepositoryEntry `yaml:"repositories"`
}

type RepositoryEntry struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// 实现 ListCharts 方法
func (s *HelmManagerServer) ListCharts(ctx context.Context, req *pb.ListChartsRequest) (*pb.ListChartsResponse, error) {
	logger.L().Info("ListCharts called", zap.String("request", req.String()))

	return &pb.ListChartsResponse{
		Charts: []*pb.ChartInfo{}, // 返回空列表
	}, nil
}
