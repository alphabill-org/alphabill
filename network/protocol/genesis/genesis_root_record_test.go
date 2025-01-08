package genesis

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/stretchr/testify/require"
)

func TestGenesisRootRecord_FindPubKeyById(t *testing.T) {
	type fields struct {
		RootValidators []*types.NodeInfo
	}
	type args struct {
		id string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *types.NodeInfo
	}{
		{
			name: "The nil test, do not crash",
			fields: fields{
				RootValidators: nil,
			},
			args: args{"test"},
			want: nil,
		},
		{
			name: "The empty list, do not crash",
			fields: fields{
				RootValidators: []*types.NodeInfo{},
			},
			args: args{"test"},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisRootRecord{
				Version:        1,
				RootValidators: tt.fields.RootValidators,
				Consensus:      nil,
			}
			if got := x.FindRootValidatorByNodeID(tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindPubKeyById() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenesisRootRecord_FindPubKeyById_Nil(t *testing.T) {
	var rg *GenesisRootRecord = nil
	require.Nil(t, rg.FindRootValidatorByNodeID("test"))
	nodeInfo := make([]*types.NodeInfo, totalNodes)
	for i := range nodeInfo {
		_, verifier := testsig.CreateSignerAndVerifier(t)
		nodeInfo[i] = trustbase.NewNodeInfoFromVerifier(fmt.Sprint(i), 1, verifier)
	}
	rg = &GenesisRootRecord{
		Version:        1,
		RootValidators: nodeInfo,
	}
	require.NotNil(t, rg.FindRootValidatorByNodeID("1"))
	require.Nil(t, rg.FindRootValidatorByNodeID("5"))
}

func TestGenesisRootRecord_IsValid_Nil(t *testing.T) {
	var rg *GenesisRootRecord = nil
	err := rg.IsValid()
	require.ErrorIs(t, err, ErrGenesisRootIssNil)
}

func TestGenesisRootRecord_IsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	_, verifier2 := testsig.CreateSignerAndVerifier(t)
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
	}
	require.NoError(t, consensus.Sign("test", signer))
	type fields struct {
		RootValidators []*types.NodeInfo
		Consensus      *ConsensusParams
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr string
	}{
		{
			name: "Root validators nil",
			fields: fields{
				RootValidators: nil,
				Consensus:      consensus,
			},
			wantErr: ErrNoRootValidators.Error(),
		},
		{
			name: "Consensus nil",
			fields: fields{
				RootValidators: []*types.NodeInfo{
					trustbase.NewNodeInfoFromVerifier("test", 1, verifier),
				},
				Consensus:      nil},
			wantErr: ErrConsensusIsNil.Error(),
		},
		{
			name: "Consensus not valid",
			fields: fields{
				RootValidators: []*types.NodeInfo{
					trustbase.NewNodeInfoFromVerifier("test", 1, verifier),
				},
				Consensus: &ConsensusParams{
					Version:             1,
					TotalRootValidators: totalNodes,
					BlockRateMs:         0,
					ConsensusTimeoutMs:  DefaultConsensusTimeout,
					HashAlgorithm:       hashAlgo}},
			wantErr: "block rate too small",
		},
		{
			name: "Not signed by validator",
			fields: fields{
				RootValidators: []*types.NodeInfo{
					trustbase.NewNodeInfoFromVerifier("test", 1, verifier),
				},
				Consensus: &ConsensusParams{
					Version:             1,
					TotalRootValidators: totalNodes,
					BlockRateMs:         blockRate,
					ConsensusTimeoutMs:  DefaultConsensusTimeout,
					HashAlgorithm:       hashAlgo}},
			wantErr: "consensus parameters is not signed by all validators",
		},
		{
			name: "Not signed by all validators",
			fields: fields{
				RootValidators: []*types.NodeInfo{
					trustbase.NewNodeInfoFromVerifier("test", 1, verifier),
					trustbase.NewNodeInfoFromVerifier("xxx", 1, verifier2),
				},
				Consensus: consensus},
			wantErr: "consensus parameters is not signed by all validators",
		},
		{
			name: "Duplicate validators",
			fields: fields{
				RootValidators: []*types.NodeInfo{
					trustbase.NewNodeInfoFromVerifier("test", 1, verifier),
					trustbase.NewNodeInfoFromVerifier("test", 1, verifier),
					trustbase.NewNodeInfoFromVerifier("test", 1, verifier),
				},
				Consensus: consensus},
			wantErr: "duplicate node id: test",
		},
		{
			name: "Unknown validator",
			fields: fields{
				RootValidators: []*types.NodeInfo{
					trustbase.NewNodeInfoFromVerifier("t", 1, verifier2),
				},
				Consensus: consensus},
			wantErr: "consensus parameters signed by unknown validator:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisRootRecord{
				Version:        1,
				RootValidators: tt.fields.RootValidators,
				Consensus:      tt.fields.Consensus,
			}
			require.ErrorContains(t, x.IsValid(), tt.wantErr)
		})
	}
}

func TestGenesisRootRecord_IsValidMissingPublicKeyInfo(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	signer2, _ := testsig.CreateSignerAndVerifier(t)
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
	}
	err := consensus.Sign("test1", signer)
	require.NoError(t, err)
	err = consensus.Sign("test2", signer2)
	require.NoError(t, err)
	x := &GenesisRootRecord{
		Version:        1,
		RootValidators: []*types.NodeInfo{trustbase.NewNodeInfoFromVerifier("test", 1, verifier)},
		Consensus:      consensus,
	}
	require.ErrorContains(t, x.IsValid(), "consensus parameters signed by unknown validator")
}

func TestGenesisRootRecord_Verify_Nil(t *testing.T) {
	var rg *GenesisRootRecord = nil
	err := rg.Verify()
	require.ErrorIs(t, err, ErrGenesisRootIssNil)
}

// Must be signed by total root validators as specified by consensus structure
func TestGenesisRootRecord_VerifyOk(t *testing.T) {
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
	}
	rootValidators := make([]*types.NodeInfo, totalNodes)
	for i := range rootValidators {
		signer, verifier := testsig.CreateSignerAndVerifier(t)
		err := consensus.Sign(fmt.Sprint(i), signer)
		require.NoError(t, err)
		rootValidators[i] = trustbase.NewNodeInfoFromVerifier(fmt.Sprint(i), 1, verifier)
	}
	var x *GenesisRootRecord
	t.Run("Verify", func(t *testing.T) {
		x = &GenesisRootRecord{
			Version:        1,
			RootValidators: rootValidators,
			Consensus:      consensus,
		}
		require.NoError(t, x.Verify())
	})

	t.Run("Serialize and verify", func(t *testing.T) {
		bs, err := types.Cbor.Marshal(x)
		require.NoError(t, err)
		x2 := &GenesisRootRecord{Version: 1}
		err = types.Cbor.Unmarshal(bs, x2)
		require.NoError(t, err)
		require.NoError(t, x2.Verify())
	})
}

func TestGenesisRootRecord_VerifyErrNoteSignedByAll(t *testing.T) {
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: totalNodes,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
	}
	rootValidators := make([]*types.NodeInfo, totalNodes)
	for i := range rootValidators {
		signer, verifier := testsig.CreateSignerAndVerifier(t)
		err := consensus.Sign(fmt.Sprint(i), signer)
		require.NoError(t, err)
		rootValidators[i] = trustbase.NewNodeInfoFromVerifier(fmt.Sprint(i), 1, verifier)
	}
	// remove one signature
	delete(consensus.Signatures, rootValidators[0].NodeID)
	x := &GenesisRootRecord{
		Version:        1,
		RootValidators: rootValidators,
		Consensus:      consensus,
	}
	require.ErrorContains(t, x.Verify(), "not signed by all")
}

func TestGenesisRootRecord_Verify(t *testing.T) {
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: totalNodes + 1,
		BlockRateMs:         blockRate,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       hashAlgo,
	}
	rootValidators := make([]*types.NodeInfo, totalNodes)
	for i := range rootValidators {
		signer, verifier := testsig.CreateSignerAndVerifier(t)
		err := consensus.Sign(fmt.Sprint(i), signer)
		require.NoError(t, err)
		rootValidators[i] = trustbase.NewNodeInfoFromVerifier(fmt.Sprint(i), 1, verifier)
	}

	type fields struct {
		RootValidators []*types.NodeInfo
		Consensus      *ConsensusParams
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr string
	}{
		{
			name: "Root validators nil",
			fields: fields{
				RootValidators: nil,
				Consensus:      consensus,
			},
			wantErr: ErrNoRootValidators.Error(),
		},
		{
			name: "Consensus nil",
			fields: fields{
				RootValidators: rootValidators,
				Consensus:      nil},
			wantErr: ErrConsensusIsNil.Error(),
		},
		{
			name: "Consensus total validators does not match public keys",
			fields: fields{
				RootValidators: rootValidators,
				Consensus:      consensus},
			wantErr: ErrRootValidatorsSize.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &GenesisRootRecord{
				Version:        1,
				RootValidators: tt.fields.RootValidators,
				Consensus:      tt.fields.Consensus,
			}
			require.ErrorContains(t, x.Verify(), tt.wantErr)
		})
	}
}
