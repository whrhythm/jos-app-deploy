package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "jos-deployment/api/v1alpha1/pb"
	podpb "jos-deployment/api/v1alpha1/pb_pod"
	"jos-deployment/pkg/helm"
	"jos-deployment/pkg/logger"
	"jos-deployment/pkg/server"
)

func init() {
	// 初始化全局Logger
	logger.InitLogger()
}

// UploadResponse REST API 响应结构
type UploadResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ChartUrl     string `json:"chart_url,omitempty"`
	SizeReceived uint64 `json:"size_received,omitempty"`
	Digest       string `json:"digest,omitempty"`
}

// handleChartUpload 处理 Chart 文件上传的 REST API
func handleChartUpload(w http.ResponseWriter, r *http.Request) {
	// 只允许 POST 请求
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}

	// 解析 multipart form
	err := r.ParseMultipartForm(32 << 20) // 32MB
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse multipart form: %v", err))
		return
	}

	// 获取上传的文件
	file, header, err := r.FormFile("chart")
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to get chart file: %v", err))
		return
	}
	defer file.Close()

	//文件名字
	fileName := header.Filename
	fmt.Println("Uploaded file name:", fileName)

	//chartName是fileName第一个'-'前面的字符串
	chartName := strings.SplitN(fileName, "-", 2)[0]

	var chartVersion string
	repoName := "library" // 默认仓库名
	//chartVersion 是 fileName第一个'-'和'.tgz'之间的字符串
	if len(strings.SplitN(fileName, "-", 2)) > 1 {
		chartVersion = strings.TrimSuffix(strings.SplitN(fileName, "-", 2)[1], ".tgz")
	} else {
		chartVersion = "1.0.0"
	}

	// 验证文件扩展名
	if filepath.Ext(header.Filename) != ".tgz" {
		writeErrorResponse(w, http.StatusBadRequest, "Only .tgz files are allowed")
		return
	}

	// 创建临时文件
	tempDir := filepath.Join(os.TempDir(), "helm-rest-uploads")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create temp directory: %v", err))
		return
	}

	chartFileName := fmt.Sprintf("%s-%s.tgz", chartName, chartVersion)
	tempFilePath := filepath.Join(tempDir, chartFileName)

	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create temp file: %v", err))
		return
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempFilePath) // 清理临时文件
	}()

	// 复制文件内容并计算 SHA256
	hasher := sha256.New()
	multiWriter := io.MultiWriter(tempFile, hasher)

	size, err := io.Copy(multiWriter, file)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save file: %v", err))
		return
	}

	tempFile.Close() // 确保文件写入完成

	// 使用 Helm 服务推送到 Harbor
	chartUrl, err := helm.PushChartToHarbor(tempFilePath, repoName, chartFileName)
	if err != nil {
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to push to Harbor: %v", err))
		return
	}
	// 返回成功响应
	response := UploadResponse{
		Success:      true,
		Message:      "Chart uploaded successfully",
		ChartUrl:     chartUrl,
		SizeReceived: uint64(size),
		Digest:       fmt.Sprintf("sha256:%x", hasher.Sum(nil)),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	logger.L().Info("Chart uploaded successfully via REST API",
		zap.String("chart_name", chartName),
		zap.String("chart_version", chartVersion),
		zap.String("repo_name", repoName),
		zap.String("file_size", fmt.Sprintf("%d", size)),
	)
}

// writeErrorResponse 写入错误响应
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := UploadResponse{
		Success: false,
		Message: message,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)

	logger.L().Error("REST API error",
		zap.Int("status_code", statusCode),
		zap.String("message", message),
	)
}

func main() {
	defer logger.Sync()
	server.Server()

	// 启动 HTTP 网关
	ctx := context.Background()
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	err := pb.RegisterHelmManagerServiceHandlerFromEndpoint(ctx, mux, "localhost:50051", opts)
	if err != nil {
		log.Fatal("Failed to register gRPC handler:", err)
	}

	err = podpb.RegisterPodManagerServiceHandlerFromEndpoint(ctx, mux, "localhost:50051", opts)
	if err != nil {
		log.Fatal("Failed to register PodManagerService handler:", err)
	}

	// 添加自定义 REST API 路由
	httpMux := http.NewServeMux()

	// 将 gRPC Gateway 路由挂载到根路径
	httpMux.Handle("/", mux)

	// 添加文件上传 REST API
	httpMux.HandleFunc("/v1alpha1/chart/upload", handleChartUpload)

	log.Println("gRPC server on :50051, HTTP gateway on :8080")
	http.ListenAndServe(":8080", httpMux)
}
