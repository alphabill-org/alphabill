package types

import "github.com/alphabill-org/alphabill/internal/network/protocol"

type (
	Block struct {
		_                  struct{} `cbor:",toarray"`
		SystemID           protocol.SystemIdentifier
		ShardID            []byte
		PreviousBlockHash  []byte
		ProposerID         string
		Transactions       []*TransactionRecord
		UnicityCertificate *UnicityCertificate
	}
)
