package replication

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/stretchr/testify/require"
)

func TestLedgerReplicationResponse_Pretty_okEmpty(t *testing.T) {
	r := &LedgerReplicationResponse{
		Status:  LedgerReplicationResponse_OK,
		Message: "",
		Blocks:  nil,
	}
	res := r.Pretty()
	require.Contains(t, res, "status")
	require.NotContains(t, res, "message")
	require.NotContains(t, res, "blocks")
}

func TestLedgerReplicationResponse_Pretty_okWithBlocks(t *testing.T) {
	r := &LedgerReplicationResponse{
		Status:  LedgerReplicationResponse_OK,
		Message: "",
		Blocks: []*block.Block{
			{
				BlockNumber: 1,
			},
			{
				BlockNumber: 2,
			},
		},
	}
	res := r.Pretty()
	require.Contains(t, res, "status")
	require.NotContains(t, res, "message")
	require.Contains(t, res, "blocks 1..2")
}

func TestLedgerReplicationResponse_Pretty_error(t *testing.T) {
	r := &LedgerReplicationResponse{
		Status:  LedgerReplicationResponse_INVALID_REQUEST_PARAMETERS,
		Message: "something bad",
		Blocks:  []*block.Block{{}},
	}
	res := r.Pretty()
	require.Contains(t, res, "status")
	require.Contains(t, res, "message:")
	require.NotContains(t, res, "blocks")
}
