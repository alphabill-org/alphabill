package handshake

import (
	"errors"

	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	ErrHandshakeIsNil     = errors.New("handshake is nil")
	ErrInvalidPartitionID = errors.New("invalid partition identifier")
	ErrMissingNodeID      = errors.New("missing node identifier")
)

type Handshake struct {
	_           struct{} `cbor:",toarray"`
	PartitionID types.PartitionID
	ShardID     types.ShardID
	NodeID      string
}

func (h *Handshake) IsValid() error {
	if h == nil {
		return ErrHandshakeIsNil
	}
	if h.PartitionID == 0 {
		return ErrInvalidPartitionID
	}
	if len(h.NodeID) == 0 {
		return ErrMissingNodeID
	}
	return nil
}
