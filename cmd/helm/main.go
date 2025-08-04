package main

import (
	"context"
	"log"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "jos-deployment/api/v1alpha1/pb"
	"jos-deployment/pkg/logger"
	"jos-deployment/pkg/server"
)

func init() {
	// 初始化全局Logger
	logger.InitLogger()
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

	log.Println("gRPC server on :50051, HTTP gateway on :8080")
	http.ListenAndServe(":8080", mux)
}
