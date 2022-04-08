package proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/transaction/. --go-grpc_out=paths=source_relative:../internal/rpc/transaction/ transaction.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/transaction/. --go-grpc_out=paths=source_relative:../internal/rpc/transaction/ moneytx.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/transaction/. --go-grpc_out=paths=source_relative:../internal/rpc/transaction/ verifiabledatatx.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/alphabill/. --go-grpc_out=paths=source_relative:../internal/rpc/alphabill/ alphabill.proto
