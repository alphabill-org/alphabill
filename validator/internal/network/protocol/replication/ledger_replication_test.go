package replication

import (
	"testing"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/stretchr/testify/require"
)

func TestLedgerReplicationResponse_Pretty_okEmpty(t *testing.T) {
	r := &LedgerReplicationResponse{
		Status:  Ok,
		Message: "",
		Blocks:  nil,
	}
	res := r.Pretty()
	require.Contains(t, res, "status")
	require.NotContains(t, res, "message")
	require.Contains(t, res, "0 blocks")
}

func TestLedgerReplicationResponse_Pretty_okWithBlocks(t *testing.T) {
	r := &LedgerReplicationResponse{
		Status:  Ok,
		Message: "",
		Blocks: []*types.Block{
			{
				UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
			},
			{
				UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 2}},
			},
		},
	}
	res := r.Pretty()
	require.Contains(t, res, "status")
	require.NotContains(t, res, "message")
	require.Contains(t, res, "2 blocks")
}

func TestLedgerReplicationResponse_Pretty_error(t *testing.T) {
	r := &LedgerReplicationResponse{
		Status:  InvalidRequestParameters,
		Message: "something bad",
		Blocks:  []*types.Block{{}},
	}
	res := r.Pretty()
	require.Contains(t, res, "status")
	require.Contains(t, res, "message:")
	require.Contains(t, res, "1 blocks")
}
