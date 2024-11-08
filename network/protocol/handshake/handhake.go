package handshake

import (
	"errors"

	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	ErrHandshakeIsNil             = errors.New("handshake is nil")
	ErrInvalidPartitionIdentifier = errors.New("invalid partition identifier")
	ErrMissingNodeIdentifier      = errors.New("missing node identifier")
)

type Handshake struct {
	_              struct{} `cbor:",toarray"`
	Partition      types.PartitionID
	Shard          types.ShardID
	NodeIdentifier string
}

func (h *Handshake) IsValid() error {
	if h == nil {
		return ErrHandshakeIsNil
	}
	if h.Partition == 0 {
		return ErrInvalidPartitionIdentifier
	}
	if len(h.NodeIdentifier) == 0 {
		return ErrMissingNodeIdentifier
	}
	return nil
}
