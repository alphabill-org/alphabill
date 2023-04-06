package handshake

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandshake_IsValid_Nil(t *testing.T) {
	var req *Handshake = nil
	require.ErrorIs(t, ErrHandshakeIsNil, req.IsValid())
}

func TestHandshake_IsValid_Error(t *testing.T) {
	h := &Handshake{
		SystemIdentifier: nil,
		NodeIdentifier:   "test",
	}
	require.ErrorIs(t, ErrInvalidSystemIdentifier, h.IsValid())
	h = &Handshake{
		SystemIdentifier: []byte{0},
		NodeIdentifier:   "test",
	}
	require.ErrorIs(t, ErrInvalidSystemIdentifier, h.IsValid())
	h = &Handshake{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "",
	}
	require.ErrorIs(t, ErrMissingNodeIdentifier, h.IsValid())
}

func TestHandshake_IsValid(t *testing.T) {
	h := &Handshake{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "test",
	}
	require.NoError(t, h.IsValid())
}
