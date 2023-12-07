package proto

//go:generate protoc -I=. --go_out=paths=source_relative:../alphabill/. --go-grpc_out=paths=source_relative:../alphabill/ alphabill.proto
