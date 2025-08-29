package node

import (
	context "context"
	"fmt"
	"os"

	pb "jos-deployment/api/v1alpha1/pb_node"

	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NodeManagerServer implements pb.NodeManagerServiceServer
// 需要在 gRPC server 注册

type NodeManagerServer struct {
	pb.UnimplementedNodeManagerServiceServer
	KubeClient *kubernetes.Clientset
}

func (s *NodeManagerServer) ListNodes(ctx context.Context, req *pb.ListNodesRequest) (*pb.ListNodesResponse, error) {
	nodeList, err := s.KubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return &pb.ListNodesResponse{Code: 1, Message: fmt.Sprintf("failed to list nodes: %v", err), Success: false}, err
	}
	var nodes []*pb.NodeInfo
	for _, n := range nodeList.Items {
		info := &pb.NodeInfo{
			Name:             n.Name,
			Status:           string(n.Status.Phase),
			OsImage:          n.Status.NodeInfo.OSImage,
			KubeletVersion:   n.Status.NodeInfo.KubeletVersion,
			ContainerRuntime: n.Status.NodeInfo.ContainerRuntimeVersion,
			Labels:           n.Labels,
			CreatedAt:        timestamppb.New(n.CreationTimestamp.Time),
		}
		for _, addr := range n.Status.Addresses {
			if addr.Type == "InternalIP" {
				info.InternalIp = addr.Address
			} else if addr.Type == "ExternalIP" {
				info.ExternalIp = addr.Address
			}
		}
		if req.Keyword == "" || (req.Keyword != "" && (contains(n.Name, req.Keyword) || contains(info.InternalIp, req.Keyword))) {
			nodes = append(nodes, info)
		}
	}
	return &pb.ListNodesResponse{Code: 0, Message: "success", Success: true, Nodes: nodes}, nil
}

func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) > 0 && (s == substr || (len(substr) > 0 && (len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || (len(s) > 2 && (s[1:len(substr)+1] == substr)))))))))
}

func (s *NodeManagerServer) AddNode(ctx context.Context, req *pb.AddNodeRequest) (*pb.AddNodeResponse, error) {
	// 使用 Cluster API：在集群中创建一个 Machine CR，cluster-api provider 会处理实际机器创建/Join。
	// 此函数创建一个最小的 Machine 对象，要求 cluster-api 和相应的 infrastructure provider 已部署并配置。

	// 必需字段检查
	if req.Name == "" {
		return &pb.AddNodeResponse{Code: 1, Message: "missing node name", Success: false}, fmt.Errorf("missing node name")
	}

	// 读取目标 cluster 名称（可通过环境变量传入），默认 "default"
	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "default"
	}

	// 获取 in-cluster rest config
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return &pb.AddNodeResponse{Code: 1, Message: fmt.Sprintf("failed to get in-cluster config: %v", err), Success: false}, err
	}

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return &pb.AddNodeResponse{Code: 1, Message: fmt.Sprintf("failed to create dynamic client: %v", err), Success: false}, err
	}

	gvr := schema.GroupVersionResource{Group: "cluster.x-k8s.io", Version: "v1beta1", Resource: "machines"}

	// 构造最小 Machine CR。注意：infrastructureRef 和 bootstrap config 通常由 provider 管理，
	// 这里只创建基础的 Machine，用户需确保 provider 能处理该 Machine。
	machine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta1",
			"kind":       "Machine",
			"metadata": map[string]interface{}{
				"name": req.Name,
			},
			"spec": map[string]interface{}{
				"clusterName": clusterName,
				// 额外字段可根据 provider 扩展，如 bootstrap, infrastructureRef 等
			},
		},
	}

	// Machines 通常位于 cluster namespace；我们创建在 "default" namespace（可配置）
	targetNS := os.Getenv("CAPI_NAMESPACE")
	if targetNS == "" {
		targetNS = "default"
	}

	_, err = dyn.Resource(gvr).Namespace(targetNS).Create(ctx, machine, metav1.CreateOptions{})
	if err != nil {
		return &pb.AddNodeResponse{Code: 1, Message: fmt.Sprintf("failed to create Machine CR: %v", err), Success: false}, err
	}

	return &pb.AddNodeResponse{Code: 0, Message: "Machine resource created (cluster-api will handle machine provisioning)", Success: true}, nil
}

func (s *NodeManagerServer) DeleteNode(ctx context.Context, req *pb.DeleteNodeRequest) (*pb.DeleteNodeResponse, error) {
	err := s.KubeClient.CoreV1().Nodes().Delete(ctx, req.Name, metav1.DeleteOptions{})
	if err != nil {
		return &pb.DeleteNodeResponse{Code: 1, Message: fmt.Sprintf("failed to delete node: %v", err), Success: false}, err
	}
	return &pb.DeleteNodeResponse{Code: 0, Message: "success", Success: true}, nil
}
