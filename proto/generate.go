package proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/transaction/. transaction.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/transaction/. --go-grpc_out=paths=source_relative:../internal/rpc/transaction/ moneytx.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/alphabill/. --go-grpc_out=paths=source_relative:../internal/rpc/alphabill/ alphabill.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/certificates/. certificates.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/protocol/p1/. p1.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/protocol/genesis/. genesis.proto
