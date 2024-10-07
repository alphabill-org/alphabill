package handshake

import (
	"errors"

	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	ErrHandshakeIsNil          = errors.New("handshake is nil")
	ErrInvalidSystemIdentifier = errors.New("invalid system identifier")
	ErrMissingNodeIdentifier   = errors.New("missing node identifier")
)

type Handshake struct {
	_              struct{} `cbor:",toarray"`
	Partition      types.SystemID
	Shard          types.ShardID
	NodeIdentifier string
}

func (h *Handshake) IsValid() error {
	if h == nil {
		return ErrHandshakeIsNil
	}
	if h.Partition == 0 {
		return ErrInvalidSystemIdentifier
	}
	if len(h.NodeIdentifier) == 0 {
		return ErrMissingNodeIdentifier
	}
	return nil
}
