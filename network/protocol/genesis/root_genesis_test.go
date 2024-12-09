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
	rootKeyInfo := &PublicKeyInfo{
		NodeID:              "1",
		SigningPublicKey:    pubKey,
		EncryptionPublicKey: pubKey,
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
			name: "invalid root node info",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					Version:        1,
					RootValidators: []*PublicKeyInfo{{NodeID: "111", SigningPublicKey: nil, EncryptionPublicKey: nil}},
					Consensus:      rootConsensus},
			},
			wantErr: ErrPubKeyInfoSigningKeyIsInvalid.Error(),
		},
		{
			name: "partitions not found",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					Version:        1,
					RootValidators: []*PublicKeyInfo{rootKeyInfo},
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
					RootValidators: []*PublicKeyInfo{rootKeyInfo},
					Consensus:      rootConsensus,
				},
				Partitions: []*GenesisPartitionRecord{
					{
						Version:              1,
						Nodes:                []*PartitionNode{{Version: 1, NodeID: "1", SigningPublicKey: nil, EncryptionPublicKey: nil, BlockCertificationRequest: nil}},
						Certificate:          nil,
						PartitionDescription: &types.PartitionDescriptionRecord{Version: 1, NetworkID: 5, PartitionID: 1, T2Timeout: time.Second},
					},
					{
						Version:              1,
						Nodes:                []*PartitionNode{{Version: 1, NodeID: "1", SigningPublicKey: nil, EncryptionPublicKey: nil, BlockCertificationRequest: nil}},
						Certificate:          nil,
						PartitionDescription: &types.PartitionDescriptionRecord{Version: 1, NetworkID: 5, PartitionID: 1, T2Timeout: time.Second},
					},
				},
			},
			wantErr: "duplicated partition identifier: ",
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
	_, encryptionPubKey := testsig.CreateSignerAndVerifier(t)
	hash := []byte{2}
	node := createPartitionNode(t, nodeID, signingKey, encryptionPubKey)
	consensus := &ConsensusParams{
		Version:             1,
		TotalRootValidators: 1,
		BlockRateMs:         MinBlockRateMs,
		ConsensusTimeoutMs:  MinBlockRateMs + MinConsensusTimeout,
		HashAlgorithm:       uint32(gocrypto.SHA256),
	}
	// create root node
	rSigner, rVerifier := testsig.CreateSignerAndVerifier(t)
	rVerifyPubKey, err := rVerifier.MarshalPublicKey()
	require.NoError(t, err)
	_, rEncryption := testsig.CreateSignerAndVerifier(t)
	rEncPubKey, err := rEncryption.MarshalPublicKey()
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
				Nodes:   []*PartitionNode{node},
				Certificate: &types.UnicityCertificate{
					Version:     1,
					InputRecord: &types.InputRecord{Version: 1},
					UnicitySeal: unicitySeal,
				},
				PartitionDescription: systemDescription,
			},
		},
	}
	require.Equal(t, hash, rg.GetRoundHash())
	require.Equal(t, uint64(2), rg.GetRoundNumber())
	require.Equal(t, 1, len(rg.GetPartitionRecords()))
	require.Equal(t,
		&PartitionRecord{
			PartitionDescription: systemDescription,
			Validators:           []*PartitionNode{node},
		},
		rg.GetPartitionRecords()[0],
	)
	require.ErrorIs(t, rg.Verify(), ErrRootGenesisRecordIsNil)
	// add root record
	rg.Root = &GenesisRootRecord{
		Version: 1,
		RootValidators: []*PublicKeyInfo{
			{NodeID: rootID, SigningPublicKey: rVerifyPubKey, EncryptionPublicKey: rEncPubKey},
		},
		Consensus: consensus,
	}
	require.ErrorContains(t, rg.Verify(), "root genesis record error: consensus parameters is not signed by all validators")
	// sign consensus
	require.NoError(t, consensus.Sign(rootID, rSigner))
	rg.Root.Consensus = consensus
	require.ErrorContains(t, rg.Verify(), "root genesis partition record 0 error:")
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
				Nodes:   []*PartitionNode{node},
				Certificate: &types.UnicityCertificate{
					Version:     1,
					InputRecord: &types.InputRecord{Version: 1},
					UnicitySeal: unicitySeal,
				},
				PartitionDescription: systemDescription,
			},
			{
				Version: 1,
				Nodes:   []*PartitionNode{node},
				Certificate: &types.UnicityCertificate{
					Version:     1,
					InputRecord: &types.InputRecord{Version: 1},
					UnicitySeal: unicitySeal,
				},
				PartitionDescription: systemDescription,
			},
		},
	}
	require.ErrorContains(t, rgDuplicatePartitions.Verify(), "root genesis duplicate partition error")

	// test CBOR marshalling
	rgBytes, err := types.Cbor.Marshal(rg)
	require.NoError(t, err)
	rg2 := &RootGenesis{Version: 1}
	require.NoError(t, types.Cbor.Unmarshal(rgBytes, rg2))
	require.Equal(t, rg, rg2)
}
