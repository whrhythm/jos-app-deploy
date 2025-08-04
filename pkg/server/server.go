package server

import (
	pb "jos-deployment/api/v1alpha1/pb"
	"jos-deployment/pkg/helm"
	"log"
	"net"

	"google.golang.org/grpc"
)

func Server() {
	// 启动 gRPC 服务
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterHelmManagerServiceServer(grpcServer, &helm.HelmManagerServer{})
	go grpcServer.Serve(lis)
}
