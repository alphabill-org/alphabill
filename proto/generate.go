package proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/. --go-grpc_out=paths=source_relative:../internal/rpc/ payment/payment.proto
