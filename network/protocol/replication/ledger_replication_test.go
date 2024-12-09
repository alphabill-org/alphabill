package replication

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
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
	uc1, err := (&types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{Version: 1, RoundNumber: 1}}).MarshalCBOR()
	require.NoError(t, err)
	uc2, err := (&types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{Version: 1, RoundNumber: 2}}).MarshalCBOR()
	require.NoError(t, err)
	r := &LedgerReplicationResponse{
		Status:  Ok,
		Message: "",
		Blocks: []*types.Block{
			{
				UnicityCertificate: uc1,
			},
			{
				UnicityCertificate: uc2,
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

func TestLedgerReplicationRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		request *LedgerReplicationRequest
		wantErr error
	}{
		{
			name:    "ValidRequest",
			request: &LedgerReplicationRequest{PartitionID: 1, NodeID: "node1", BeginBlockNumber: 1, EndBlockNumber: 10},
			wantErr: nil,
		},
		{
			name:    "ZeroBlockNumbers",
			request: &LedgerReplicationRequest{PartitionID: 1, NodeID: "node1"},
			wantErr: nil,
		},
		{
			name:    "EqualBlockNumbers",
			request: &LedgerReplicationRequest{PartitionID: 1, NodeID: "node1", BeginBlockNumber: 1, EndBlockNumber: 1},
			wantErr: nil,
		},
		{
			name:    "NilRequest",
			request: nil,
			wantErr: ErrLedgerReplicationReqIsNil,
		},
		{
			name:    "InvalidPartitionID",
			request: &LedgerReplicationRequest{PartitionID: 0, NodeID: "node1", BeginBlockNumber: 1, EndBlockNumber: 10},
			wantErr: ErrInvalidPartitionID,
		},
		{
			name:    "MissingNodeID",
			request: &LedgerReplicationRequest{PartitionID: 1, NodeID: "", BeginBlockNumber: 1, EndBlockNumber: 10},
			wantErr: ErrNodeIDIsMissing,
		},
		{
			name:    "InvalidBlockRange",
			request: &LedgerReplicationRequest{PartitionID: 1, NodeID: "node1", BeginBlockNumber: 10, EndBlockNumber: 1},
			wantErr: fmt.Errorf("invalid block request range from 10 to 1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.IsValid()
			require.Equal(t, tt.wantErr, err)
		})
	}
}

func TestLedgerReplicationResponseValidation(t *testing.T) {
	tests := []struct {
		name     string
		response *LedgerReplicationResponse
		wantErr  error
	}{
		{
			name:     "ValidResponse",
			response: &LedgerReplicationResponse{Status: Ok, Blocks: []*types.Block{}},
			wantErr:  nil,
		},
		{
			name:     "NilResponse",
			response: nil,
			wantErr:  ErrLedgerReplicationRespIsNil,
		},
		{
			name:     "BlocksNilWithOkStatus",
			response: &LedgerReplicationResponse{Status: Ok, Blocks: nil},
			wantErr:  ErrLedgerResponseBlocksIsNil,
		},
		{
			name:     "ValidWithMessage",
			response: &LedgerReplicationResponse{Status: Ok, Message: "message", Blocks: []*types.Block{}},
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.IsValid()
			require.Equal(t, tt.wantErr, err)
		})
	}
}
