package server

import (
	pb "jos-deployment/api/v1alpha1/pb"
	podpb "jos-deployment/api/v1alpha1/pb_pod"
	"jos-deployment/pkg/helm"
	"jos-deployment/pkg/pod"
	"log"
	"net"

	"google.golang.org/grpc"
)

func Server() {
	grpcServer := grpc.NewServer()
	pb.RegisterHelmManagerServiceServer(grpcServer, &helm.HelmManagerServer{})
	podpb.RegisterPodManagerServiceServer(grpcServer, &pod.PodManagerServer{})
	// 启动 gRPC 服务
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}

	go grpcServer.Serve(lis)
}
