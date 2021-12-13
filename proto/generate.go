package proto

// Generates go files to ../internal/rpc/* from alphabill.proto
//go:generate protoc --go_out=paths=source_relative:../internal/rpc/. --go-grpc_out=paths=source_relative:../internal/rpc/ ./alphabill.proto
