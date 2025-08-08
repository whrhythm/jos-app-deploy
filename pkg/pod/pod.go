package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "jos-deployment/api/v1alpha1/pb_pod"
	"jos-deployment/pkg/logger"

	"jos-deployment/pkg/helm"

	"github.com/prometheus/common/model"
	"go.uber.org/zap"
)

const (
	prometheusURL = "http://192.168.20.122:32437"
)

type PodManagerServer struct {
	pb.UnimplementedPodManagerServiceServer
}

type PodMetrics struct {
	CPUCores      float64
	MemoryMB      float64
	CPUPercent    float64
	MemoryPercent float64
}

type PrometheusResponse struct {
	Status string         `json:"status"`
	Data   PrometheusData `json:"data"`
}

type PrometheusData struct {
	ResultType model.ValueType `json:"resultType"`
	Result     model.Vector    `json:"result"`
}

// DeletePod 实现删除 Pod 的 RPC 方法
func (s *PodManagerServer) DeletePod(ctx context.Context, req *pb.DeletePodRequest) (*pb.DeletePodResponse, error) {
	logger.L().Info("DeletePod called", zap.String("request", req.String()))

	// 在这里实现实际的 Pod 删除逻辑
	// 示例模拟操作
	deletionTime := timestamppb.Now()

	return &pb.DeletePodResponse{
		Message:           "Pod deleted successfully",
		DeletionTimestamp: deletionTime,
	}, nil
}

// GetPodLogs 实现获取 Pod 日志的流式 RPC 方法
func (s *PodManagerServer) GetPodLogs(req *pb.GetPodLogsRequest, stream pb.PodManagerService_GetPodLogsServer) error {
	log.Printf("Received GetPodLogs request for %s/%s", req.Namespace, req.PodName)

	// 模拟日志数据
	logs := []string{
		"Starting container...",
		"Container started successfully",
		"Application initialized",
		"Listening on port 8080",
	}

	// 流式发送日志
	for _, line := range logs {
		if err := stream.Send(&pb.LogChunk{
			Content: []byte(line + "\n"),
		}); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond) // 模拟日志间隔
	}

	return nil
}

// ConfigureHorizontalAutoscaling 实现配置 HPA 的 RPC 方法
func (s *PodManagerServer) ConfigureHorizontalAutoscaling(ctx context.Context, req *pb.ConfigureHPARequest) (*pb.ConfigureHPAResponse, error) {
	logger.L().Info("ConfigureHorizontalAutoscaling called", zap.String("request", req.String()))

	return &pb.ConfigureHPAResponse{
		Message:   "HPA configured successfully",
		CreatedAt: timestamppb.Now(),
	}, nil
}

// ConfigureVerticalAutoscaling 实现配置 VPA 的 RPC 方法
func (s *PodManagerServer) ConfigureVerticalAutoscaling(ctx context.Context, req *pb.ConfigureVPARequest) (*pb.ConfigureVPAResponse, error) {
	log.Printf("Configuring VPA for %s/%s", req.Namespace, req.DeploymentName)

	return &pb.ConfigureVPAResponse{
		Message:   "VPA configured successfully",
		CreatedAt: timestamppb.Now(),
	}, nil
}

// CreateCanaryDeployment 实现金丝雀发布的 RPC 方法
func (s *PodManagerServer) CreateCanaryDeployment(ctx context.Context, req *pb.CreateCanaryRequest) (*pb.CreateCanaryResponse, error) {
	log.Printf("Creating canary deployment for %s/%s", req.Namespace, req.RolloutName)

	return &pb.CreateCanaryResponse{
		Message:   "Canary deployment created successfully",
		CreatedAt: timestamppb.Now(),
	}, nil
}

// CreateBlueGreenDeployment 实现蓝绿发布的 RPC 方法
func (s *PodManagerServer) CreateBlueGreenDeployment(ctx context.Context, req *pb.CreateBlueGreenRequest) (*pb.CreateBlueGreenResponse, error) {
	log.Printf("Creating blue-green deployment for %s/%s", req.Namespace, req.RolloutName)

	return &pb.CreateBlueGreenResponse{
		Message:   "Blue-green deployment created successfully",
		CreatedAt: timestamppb.Now(),
	}, nil
}

func queryPrometheus(query string) (model.Value, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	params := url.Values{}
	params.Add("query", query)

	resp, err := client.Get(fmt.Sprintf("%s/api/v1/query?%s", prometheusURL, params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("error querying Prometheus: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	return result.Data.Result, nil
}

func getPodMetrics(namespace, podName string) (*PodMetrics, error) {
	metrics := &PodMetrics{}

	// CPU使用量（核）
	cpuQuery := fmt.Sprintf(
		`sum(rate(container_cpu_usage_seconds_total{namespace="%s", pod="%s"}[5m])) by (pod)`,
		namespace, podName,
	)
	cpuResult, err := queryPrometheus(cpuQuery)
	if err != nil {
		return nil, fmt.Errorf("error querying CPU usage: %v", err)
	}

	if vector, ok := cpuResult.(model.Vector); ok && len(vector) > 0 {
		for _, v := range vector {
			metrics.CPUCores += float64(v.Value)
		}
	}

	// 内存使用量（MB）
	memQuery := fmt.Sprintf(
		`sum(container_memory_working_set_bytes{namespace="%s", pod="%s"}) by (pod)`,
		namespace, podName,
	)
	memResult, err := queryPrometheus(memQuery)
	if err != nil {
		return nil, fmt.Errorf("error querying memory usage: %v", err)
	}

	if vector, ok := memResult.(model.Vector); ok && len(vector) > 0 {
		for _, v := range vector {
			metrics.MemoryMB += float64(v.Value) / 1024 / 1024 // 转换为 MB
		}
	}

	return metrics, nil
}

func (s *PodManagerServer) PodsMetrics(ctx context.Context, req *pb.PodsMetricsRequest) (*pb.PodsMetricsResponse, error) {
	logger.L().Info("PodsMetrics called", zap.String("request", req.String()))
	podList, err := helm.GetPodList(ctx, req.GetNamespace(), req.GetReleaseName())
	if err != nil {
		logger.L().Error("Failed to get pod list", zap.Error(err))
		return nil, status.Errorf(status.Code(err), "Failed to get pod list: %v", err)
	}

	var totalCPU, totalMemory float64
	totalCPU = 0.0
	totalMemory = 0.0
	for _, pod := range podList.Items {
		metrics, err := getPodMetrics(req.GetNamespace(), pod.Name)
		if err != nil {
			logger.L().Error("Failed to get pod metrics", zap.String("pod", pod.Name), zap.Error(err))
			return nil, status.Errorf(status.Code(err), "Failed to get metrics for pod %s: %v", pod.Name, err)
		}
		totalCPU += metrics.CPUCores
		totalMemory += metrics.MemoryMB
	}

	return &pb.PodsMetricsResponse{
		Code:    0,
		Success: true,
		Message: "Metrics retrieved successfully",
		Data: &pb.PodMetricsData{
			AppNum:   1,
			PodNum:   int32(len(podList.Items)),
			CpuUsage: fmt.Sprintf("%.2f cores", totalCPU),
			MemUsage: fmt.Sprintf("%.2f MB", totalMemory),
		},
	}, nil
}
