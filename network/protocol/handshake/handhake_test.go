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
		PartitionID: 0,
		NodeID:      "test",
	}
	require.ErrorIs(t, ErrInvalidPartitionID, h.IsValid())

	h = &Handshake{
		PartitionID: 1,
		NodeID:      "",
	}
	require.ErrorIs(t, ErrMissingNodeID, h.IsValid())
}

func TestHandshake_IsValid(t *testing.T) {
	h := &Handshake{
		PartitionID: 1,
		NodeID:      "test",
	}
	require.NoError(t, h.IsValid())
}
