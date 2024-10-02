package genesis

import (
	gocrypto "crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestRootGenesis_IsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	rootKeyInfo := &PublicKeyInfo{
		NodeIdentifier:      "1",
		SigningPublicKey:    pubKey,
		EncryptionPublicKey: pubKey,
	}
	rootConsensus := &ConsensusParams{
		TotalRootValidators: 1,
		BlockRateMs:         MinBlockRateMs,
		ConsensusTimeoutMs:  DefaultConsensusTimeout,
		HashAlgorithm:       uint32(gocrypto.SHA256),
		Signatures:          make(map[string][]byte),
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
					RootValidators: []*PublicKeyInfo{{NodeIdentifier: "111", SigningPublicKey: nil, EncryptionPublicKey: nil}},
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
					RootValidators: []*PublicKeyInfo{rootKeyInfo},
					Consensus:      rootConsensus,
				},
				Partitions: nil,
			},
			wantErr: ErrPartitionsNotFound.Error(),
		},
		{
			name: "genesis partition record duplicate system id",
			args: args{
				verifier: verifier,
			},
			fields: fields{
				Root: &GenesisRootRecord{
					RootValidators: []*PublicKeyInfo{rootKeyInfo},
					Consensus:      rootConsensus,
				},
				Partitions: []*GenesisPartitionRecord{
					{
						Nodes:                []*PartitionNode{{NodeIdentifier: "1", SigningPublicKey: nil, EncryptionPublicKey: nil, BlockCertificationRequest: nil}},
						Certificate:          nil,
						PartitionDescription: &types.PartitionDescriptionRecord{SystemIdentifier: 1, T2Timeout: time.Second},
					},
					{
						Nodes:                []*PartitionNode{{NodeIdentifier: "1", SigningPublicKey: nil, EncryptionPublicKey: nil, BlockCertificationRequest: nil}},
						Certificate:          nil,
						PartitionDescription: &types.PartitionDescriptionRecord{SystemIdentifier: 1, T2Timeout: time.Second},
					},
				},
			},
			wantErr: "duplicated system identifier: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &RootGenesis{
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
	node := createPartitionNode(t, nodeIdentifier, signingKey, encryptionPubKey)
	consensus := &ConsensusParams{
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
	unicitySeal := &types.UnicitySeal{Version: 1,
		PreviousHash:         make([]byte, 32),
		RootChainRoundNumber: 2,
		Timestamp:            1000,
		Hash:                 hash,
	}
	require.NoError(t, unicitySeal.Sign(rootID, rSigner))
	rg := &RootGenesis{
		Partitions: []*GenesisPartitionRecord{
			{
				Nodes: []*PartitionNode{node},
				Certificate: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{},
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
		RootValidators: []*PublicKeyInfo{
			{NodeIdentifier: rootID, SigningPublicKey: rVerifyPubKey, EncryptionPublicKey: rEncPubKey},
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
		Root:       rg.Root,
		Partitions: []*GenesisPartitionRecord{},
	}
	require.ErrorIs(t, rgNoPartitions.Verify(), ErrPartitionsNotFound)
	// duplicate partition
	rgDuplicatePartitions := &RootGenesis{
		Root: rg.Root,
		Partitions: []*GenesisPartitionRecord{
			{
				Nodes: []*PartitionNode{node},
				Certificate: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{},
					UnicitySeal: unicitySeal,
				},
				PartitionDescription: systemDescription,
			},
			{
				Nodes: []*PartitionNode{node},
				Certificate: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{},
					UnicitySeal: unicitySeal,
				},
				PartitionDescription: systemDescription,
			},
		},
	}
	require.ErrorContains(t, rgDuplicatePartitions.Verify(), "root genesis duplicate partition error")
}
