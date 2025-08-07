package exm

import (
	"context"
	"jos-deployment/example"
)

// ExampleServiceServer is the server API for ExampleService service.

type ExampleServiceServer struct {
	example.UnimplementedUserServiceServer
}

func (s *ExampleServiceServer) GetUser(ctx context.Context, req *example.GetUserRequest) (*example.User, error) {
	// 这里可以添加业务逻辑
	user := &example.User{
		Name: req.UserId,
	}
	return user, nil
}
