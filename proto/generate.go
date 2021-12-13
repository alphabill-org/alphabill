package proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/. --go-grpc_out=paths=source_relative:../internal/rpc/ payment/payment.proto
//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/ledger/. --go-grpc_out=paths=source_relative:../internal/rpc/ledger/ ledger.proto
//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/transaction/. --go-grpc_out=paths=source_relative:../internal/rpc/transaction/ transaction.proto
