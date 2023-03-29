package handshake

import "errors"

var (
	ErrHandshakeIsNil          = errors.New("handshake is nil")
	ErrInvalidSystemIdentifier = errors.New("invalid system identifier")
	ErrMissingNodeIdentifier   = errors.New("missing node identifier")
)

func (h *Handshake) IsValid() error {
	if h == nil {
		return ErrHandshakeIsNil
	}
	if len(h.SystemIdentifier) != 4 {
		return ErrInvalidSystemIdentifier
	}
	if len(h.NodeIdentifier) == 0 {
		return ErrMissingNodeIdentifier
	}
	return nil
}
