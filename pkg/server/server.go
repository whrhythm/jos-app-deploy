package server

import (
	"context"
	"fmt"
	pb "jos-deployment/api/v1alpha1/pb"
	podpb "jos-deployment/api/v1alpha1/pb_pod"
	routepb "jos-deployment/api/v1alpha1/pb_routes"
	"jos-deployment/pkg/helm"
	"jos-deployment/pkg/pod"
	"jos-deployment/pkg/routes"
	"log"
	"net"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// JWTInterceptor 结构体封装 JWT 拦截器相关配置
type JWTInterceptor struct {
	// 可以在此添加配置项，如白名单路径等
}

// NewJWTInterceptor 创建新的 JWT 拦截器实例
func NewJWTInterceptor() *JWTInterceptor {
	return &JWTInterceptor{}
}

// Interceptor 实现 gRPC 一元拦截器接口
func (i *JWTInterceptor) Interceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 1. 从上下文中获取元数据
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Code(10401), "missing metadata")
		}

		// 2. 检查并获取 Authorization 头
		authHeaders := md.Get("authorization")
		if len(authHeaders) == 0 {
			return nil, status.Error(codes.Code(10401), "missing metadata")
		}

		// 3. 提取 Bearer token
		tokenString := strings.TrimPrefix(authHeaders[0], "Bearer ")
		if tokenString == "" {
			return nil, status.Error(codes.Code(10401), "missing metadata")
		}

		// 4. 创建不验证签名的 JWT 解析器
		parser := jwt.NewParser(
			jwt.WithoutClaimsValidation(),
			jwt.WithValidMethods([]string{"HS256", "HS384", "HS512", "RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}),
		)

		// 5. 解析但不验证 JWT token
		token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
		if err != nil {
			return nil, status.Error(codes.Code(10401), fmt.Sprintf("invalid token format: %v", err))
		}

		// 6. 将解析后的 token 存入上下文，供后续处理使用
		ctx = context.WithValue(ctx, "jwtToken", token)

		// 7. 调用后续处理程序
		return handler(ctx, req)
	}
}

func Server() {
	// 创建拦截器实例
	jwtInterceptor := NewJWTInterceptor()
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(jwtInterceptor.Interceptor()))
	pb.RegisterHelmManagerServiceServer(grpcServer, &helm.HelmManagerServer{})
	podpb.RegisterPodManagerServiceServer(grpcServer, &pod.PodManagerServer{})
	routepb.RegisterAPISIXGatewayServiceServer(grpcServer, &routes.RoutesManageService{})
	// 启动 gRPC 服务
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}

	go grpcServer.Serve(lis)
}
