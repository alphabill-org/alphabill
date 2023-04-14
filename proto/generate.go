package proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/txsystem/. transaction.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/txsystem/fc/transactions/. fee_credit_txs.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/txsystem/money/. money_tx.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/txsystem/tokens/. token_tx.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/txsystem/sc/. sc_attributes.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/rpc/alphabill/. --go-grpc_out=paths=source_relative:../internal/rpc/alphabill/ alphabill.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/certificates/. certificates.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/block/. block.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/block/. block_proof.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../pkg/wallet/backend/bp/. bills.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/network/protocol/certification/. certification.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/network/protocol/replication/. ledger_replication.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/network/protocol/blockproposal/. block_proposal.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/network/protocol/genesis/. genesis.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/network/protocol/handshake/. handshake.proto

//go:generate protoc -I=. --go_out=paths=source_relative:../internal/network/protocol/ab_consensus/. ab_consensus.proto
