package genesis

import (
	gocrypto "crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestRootGenesis_IsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	rootKeyInfo := &types.NodeInfo{
		NodeID: "1",
		SigKey: pubKey,
	}
	rootConsensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: 1,
		BlockRateMs:         MinBlockRateMs,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       uint32(gocrypto.SHA256),
		Signatures:          make(map[string]hex.Bytes),
	}
	err = rootConsensus.Sign("1", signer)
	require.NoError(t, err)
	type fields struct {
		Root          *GenesisRootRecord
		Partitions    []*GenesisPartitionRecord
		HashAlgorithm uint32
	}
	type args struct {
		verifier crypto.Verifier
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr string
	}{
		{
			name: "root is nil",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: nil,
			},
			wantErr: ErrRootGenesisRecordIsNil.Error(),
		},
		{
			name: "invalid root validator info",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					Version:        1,
					RootValidators: []*types.NodeInfo{{NodeID: "111", SigKey: nil}},
					Consensus:      rootConsensus},
			},
			wantErr: "signing key is empty",
		},
		{
			name: "partitions not found",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					Version:        1,
					RootValidators: []*types.NodeInfo{rootKeyInfo},
					Consensus:      rootConsensus,
				},
				Partitions: nil,
			},
			wantErr: ErrPartitionsNotFound.Error(),
		},
		{
			name: "genesis partition record duplicate partition id",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					Version:        1,
					RootValidators: []*types.NodeInfo{rootKeyInfo},
					Consensus:      rootConsensus,
				},
				Partitions: []*GenesisPartitionRecord{
					{
						Version:              1,
						Validators:           []*PartitionNode{{Version: 1, NodeID: "1", SignKey: nil, BlockCertificationRequest: nil}},
						Certificate:          nil,
						PartitionDescription: &types.PartitionDescriptionRecord{Version: 1, NetworkID: 5, PartitionID: 1, T2Timeout: time.Second},
					},
					{
						Version:              1,
						Validators:           []*PartitionNode{{Version: 1, NodeID: "1", SignKey: nil, BlockCertificationRequest: nil}},
						Certificate:          nil,
						PartitionDescription: &types.PartitionDescriptionRecord{Version: 1, NetworkID: 5, PartitionID: 1, T2Timeout: time.Second},
					},
				},
			},
			wantErr: "duplicate partition identifier: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &RootGenesis{
				Version:    1,
				Root:       tt.fields.Root,
				Partitions: tt.fields.Partitions,
			}
			err := x.IsValid()
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestRootGenesis_IsValid_Nil(t *testing.T) {
	var rg *RootGenesis = nil
	err := rg.IsValid()
	require.ErrorContains(t, err, ErrRootGenesisIsNil.Error())
}

func TestRootGenesis_Verify_Nil(t *testing.T) {
	var rg *RootGenesis = nil
	err := rg.Verify()
	require.ErrorContains(t, err, ErrRootGenesisIsNil.Error())
}

func TestRootGenesis(t *testing.T) {
	signingKey, _ := testsig.CreateSignerAndVerifier(t)
	hash := []byte{2}
	node := createPartitionNode(t, nodeID, signingKey)
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: 1,
		BlockRateMs:         MinBlockRateMs,
		ConsensusTimeoutMs:  MinBlockRateMs + MinConsensusTimeout,
		HashAlgorithm:       uint32(gocrypto.SHA256),
	}
	// create root node
	rSigner, rVerifier := testsig.CreateSignerAndVerifier(t)
	rSignKey, err := rVerifier.MarshalPublicKey()
	require.NoError(t, err)

	rootID := "root"
	unicitySeal := &types.UnicitySeal{
		Version:              1,
		PreviousHash:         make([]byte, 32),
		RootChainRoundNumber: 2,
		Timestamp:            1000,
		Hash:                 hash,
	}
	require.NoError(t, unicitySeal.Sign(rootID, rSigner))
	rg := &RootGenesis{
		Version: 1,
		Partitions: []*GenesisPartitionRecord{
			{
				Version: 1,
				Validators: []*PartitionNode{node},
				Certificate: &types.UnicityCertificate{
					Version:     1,
					InputRecord: &types.InputRecord{Version: 1},
					UnicitySeal: unicitySeal,
				},
				PartitionDescription: validPDR,
			},
		},
	}
	require.Equal(t, hash, rg.GetRoundHash())
	require.Equal(t, uint64(2), rg.GetRoundNumber())
	require.ErrorIs(t, rg.Verify(), ErrRootGenesisRecordIsNil)
	// add root record
	rg.Root = &GenesisRootRecord{
		Version: 1,
		RootValidators: []*types.NodeInfo{
			{NodeID: rootID, Stake: 1, SigKey: rSignKey},
		},
		Consensus: consensus,
	}
	require.ErrorContains(t, rg.Verify(), "invalid root record: consensus parameters is not signed by all validators")
	// sign consensus
	require.NoError(t, consensus.Sign(rootID, rSigner))
	rg.Root.Consensus = consensus
	require.ErrorContains(t, rg.Verify(), "invalid partition record at index 0:")
	// no partitions
	rgNoPartitions := &RootGenesis{
		Version:    1,
		Root:       rg.Root,
		Partitions: []*GenesisPartitionRecord{},
	}
	require.ErrorIs(t, rgNoPartitions.Verify(), ErrPartitionsNotFound)
	// duplicate partition
	rgDuplicatePartitions := &RootGenesis{
		Version: 1,
		Root:    rg.Root,
		Partitions: []*GenesisPartitionRecord{
			{
				Version: 1,
				Validators: []*PartitionNode{node},
				Certificate: &types.UnicityCertificate{
					Version:     1,
					InputRecord: &types.InputRecord{Version: 1},
					UnicitySeal: unicitySeal,
				},
				PartitionDescription: validPDR,
			},
			{
				Version: 1,
				Validators: []*PartitionNode{node},
				Certificate: &types.UnicityCertificate{
					Version:     1,
					InputRecord: &types.InputRecord{Version: 1},
					UnicitySeal: unicitySeal,
				},
				PartitionDescription: validPDR,
			},
		},
	}
	require.ErrorContains(t, rgDuplicatePartitions.Verify(), "invalid partitions: duplicate partition identifier")

	// test CBOR marshalling
	rgBytes, err := types.Cbor.Marshal(rg)
	require.NoError(t, err)
	rg2 := &RootGenesis{Version: 1}
	require.NoError(t, types.Cbor.Unmarshal(rgBytes, rg2))
	require.ErrorContains(t, rg2.Verify(), "invalid partition record at index 0:")
	require.Equal(t, rg, rg2)
}
