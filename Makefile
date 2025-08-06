# Makefile for gRPC file upload demo

.PHONY: proto server client clean run-server run-client

# 生成 protobuf 代码
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative \
		helm_service.proto


# 运行服务端
run-server: server
	./bin/server

# 运行客户端（需要先准备测试文件）
run-client: client
	@echo "请确保有测试文件 example-chart-1.0.0.tgz 在当前目录"
	./bin/client

# 创建示例 chart 文件（用于测试）
create-test-chart:
	@echo "Creating test chart package..."
	helm create test-chart
	helm package test-chart
	mv test-chart-0.1.0.tgz example-chart-1.0.0.tgz
	rm -rf test-chart

# 清理编译文件
clean:
	rm -rf bin/
	rm -f example-chart-*.tgz

# 完整测试流程
test: clean create-test-chart server client
	@echo "=== 启动 gRPC 服务器 ==="
	./bin/server &
	@sleep 2
	@echo "=== 运行客户端上传测试 ==="
	./bin/client
	@echo "=== 测试完成 ==="
	@pkill -f "./bin/server" || true
