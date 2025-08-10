protoc -I . --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. --grpc-gateway_out=paths=source_relative:. helm_service.proto

protoc -I . --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. --grpc-gateway_out=paths=source_relative:. pod_service.proto

protoc -I . --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. --grpc-gateway_out=paths=source_relative:. user_service.proto

protoc -I . --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. --grpc-gateway_out=paths=source_relative:. routes_service.proto
