package genesis

import (
	gocrypto "crypto"
	"fmt"
	"testing"
	"time"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	pg "github.com/alphabill-org/alphabill/partition/genesis"
	"github.com/stretchr/testify/require"
)

func getPublicKeyAndVerifier(signer abcrypto.Signer) ([]byte, abcrypto.Verifier, error) {
	if signer == nil {
		return nil, nil, ErrSignerIsNil
	}
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, nil, err
	}
	pubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, nil, err
	}
	return pubKey, verifier, nil
}

func createPartition(t *testing.T, systemIdentifier types.SystemID, nodeID string, partitionSigner abcrypto.Signer) *genesis.PartitionRecord {
	t.Helper()
	req := createInputRequest(t, systemIdentifier, nodeID, partitionSigner)
	pubKey, _, err := getPublicKeyAndVerifier(partitionSigner)
	require.NoError(t, err)

	return &genesis.PartitionRecord{
		PartitionDescription: &types.PartitionDescriptionRecord{
			NetworkIdentifier: 5,
			SystemIdentifier:  systemIdentifier,
			TypeIdLen:         8,
			UnitIdLen:         256,
			T2Timeout:         2500 * time.Millisecond,
		},
		Validators: []*genesis.PartitionNode{{
			Version:                   1,
			NodeIdentifier:            nodeID,
			SigningPublicKey:          pubKey,
			EncryptionPublicKey:       pubKey,
			BlockCertificationRequest: req,
		}},
	}
}

func createPartitionNode(t *testing.T, systemIdentifier types.SystemID, nodeID string, partitionSigner abcrypto.Signer) *genesis.PartitionNode {
	t.Helper()
	req := createInputRequest(t, systemIdentifier, nodeID, partitionSigner)
	pubKey, _, err := getPublicKeyAndVerifier(partitionSigner)
	require.NoError(t, err)

	return &genesis.PartitionNode{
		Version:                   1,
		NodeIdentifier:            nodeID,
		SigningPublicKey:          pubKey,
		EncryptionPublicKey:       pubKey,
		BlockCertificationRequest: req,
		PartitionDescription: types.PartitionDescriptionRecord{
			NetworkIdentifier: 5,
			SystemIdentifier:  systemIdentifier,
			TypeIdLen:         8,
			UnitIdLen:         256,
			T2Timeout:         2500 * time.Millisecond,
		},
	}
}

func createInputRequest(t *testing.T, systemIdentifier types.SystemID, nodeID string, partitionSigner abcrypto.Signer) *certification.BlockCertificationRequest {
	t.Helper()
	req := &certification.BlockCertificationRequest{
		Partition:      systemIdentifier,
		NodeIdentifier: nodeID,
		InputRecord: &types.InputRecord{
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    make([]byte, 32),
			SummaryValue: []byte{1, 0, 0},
			RoundNumber:  pg.PartitionRoundNumber,
		},
	}

	err := req.Sign(partitionSigner)
	require.NoError(t, err)
	return req
}

func Test_rootGenesisConf_isValid(t *testing.T) {
	sig, ver := testsig.CreateSignerAndVerifier(t)
	pubKey, err := ver.MarshalPublicKey()
	require.NoError(t, err)
	type fields struct {
		peerID                string
		encryptionPubKeyBytes []byte
		signer                abcrypto.Signer
		totalValidators       uint32
		blockRateMs           uint32
		consensusTimeoutMs    uint32
		hashAlgorithm         gocrypto.Hash
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr string
	}{
		{
			name: "ok",
			fields: fields{
				peerID:                "1",
				encryptionPubKeyBytes: pubKey,
				signer:                sig,
				totalValidators:       1,
				blockRateMs:           genesis.MinBlockRateMs,
				consensusTimeoutMs:    genesis.MinBlockRateMs + genesis.MinConsensusTimeout,
				hashAlgorithm:         gocrypto.SHA256,
			},
		},
		{
			name: "no peer id",
			fields: fields{
				peerID:                "",
				encryptionPubKeyBytes: pubKey,
				signer:                sig,
				totalValidators:       1,
				blockRateMs:           genesis.MinBlockRateMs,
				consensusTimeoutMs:    genesis.MinConsensusTimeout,
				hashAlgorithm:         gocrypto.SHA256,
			},
			wantErr: genesis.ErrNodeIdentifierIsEmpty.Error(),
		},
		{
			name: "no pub key",
			fields: fields{
				peerID:             "1",
				signer:             sig,
				totalValidators:    1,
				blockRateMs:        genesis.MinBlockRateMs,
				consensusTimeoutMs: genesis.MinConsensusTimeout,
				hashAlgorithm:      gocrypto.SHA256,
			},
			wantErr: ErrEncryptionPubKeyIsNil.Error(),
		},
		{
			name: "no signer",
			fields: fields{
				peerID:                "1",
				encryptionPubKeyBytes: pubKey,
				totalValidators:       1,
				blockRateMs:           genesis.MinBlockRateMs,
				consensusTimeoutMs:    genesis.MinConsensusTimeout,
				hashAlgorithm:         gocrypto.SHA256,
			},
			wantErr: ErrSignerIsNil.Error(),
		},
		{
			name: "invalid validators",
			fields: fields{
				peerID:                "1",
				encryptionPubKeyBytes: pubKey,
				signer:                sig,
				totalValidators:       0,
				blockRateMs:           genesis.MinBlockRateMs,
				consensusTimeoutMs:    genesis.MinConsensusTimeout,
				hashAlgorithm:         gocrypto.SHA256,
			},
			wantErr: genesis.ErrInvalidNumberOfRootValidators.Error(),
		},
		{
			name: "invalid consensus timeout",
			fields: fields{
				peerID:                "1",
				encryptionPubKeyBytes: pubKey,
				signer:                sig,
				totalValidators:       1,
				blockRateMs:           genesis.MinBlockRateMs,
				consensusTimeoutMs:    genesis.MinConsensusTimeout - 1,
				hashAlgorithm:         gocrypto.SHA256,
			},
			wantErr: fmt.Sprintf("invalid consensus timeout, must be at least %v", genesis.MinConsensusTimeout),
		},
		{
			name: "invalid block rate",
			fields: fields{
				peerID:                "1",
				encryptionPubKeyBytes: pubKey,
				signer:                sig,
				totalValidators:       1,
				blockRateMs:           genesis.MinBlockRateMs - 1,
				consensusTimeoutMs:    genesis.DefaultConsensusTimeout,
				hashAlgorithm:         gocrypto.SHA256,
			},
			wantErr: fmt.Sprintf("invalid block rate, must be at least %v", genesis.MinBlockRateMs),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &rootGenesisConf{
				peerID:                tt.fields.peerID,
				encryptionPubKeyBytes: tt.fields.encryptionPubKeyBytes,
				signer:                tt.fields.signer,
				totalValidators:       tt.fields.totalValidators,
				blockRateMs:           tt.fields.blockRateMs,
				consensusTimeoutMs:    tt.fields.consensusTimeoutMs,
				hashAlgorithm:         tt.fields.hashAlgorithm,
			}
			err = c.isValid()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewGenesis_Ok(t *testing.T) {
	const id types.SystemID = 1
	partitionSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	partition := createPartition(t, id, "1", partitionSigner)
	require.NoError(t, err)
	rootChainSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	_, verifier := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	rg, ps, err := NewRootGenesis("test", rootChainSigner, rootPubKeyBytes, []*genesis.PartitionRecord{partition})
	require.NoError(t, err)
	require.NotNil(t, rg)
	require.NotNil(t, ps)
	require.Equal(t, 1, len(ps))
	require.NoError(t, rg.IsValid())
}

func TestNewGenesis_ConsensusNotPossible(t *testing.T) {
	const id types.SystemID = 1
	partitionSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	partitionSigner2, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	partition := createPartition(t, id, "1", partitionSigner)

	req := createInputRequest(t, id, "2", partitionSigner2)
	req.InputRecord.Hash = []byte{1, 1, 1, 1}
	req.InputRecord.BlockHash = []byte{2, 2, 2, 2}
	require.NoError(t, req.Sign(partitionSigner2))
	pubKey, _, err := getPublicKeyAndVerifier(partitionSigner2)
	require.NoError(t, err)
	pr := &genesis.PartitionNode{
		Version:                   1,
		NodeIdentifier:            "2",
		SigningPublicKey:          pubKey,
		EncryptionPublicKey:       pubKey,
		BlockCertificationRequest: req,
	}
	partition.Validators = append(partition.Validators, pr)

	rootChainSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	_, _, err = NewRootGenesis("test", rootChainSigner, rootPubKeyBytes, []*genesis.PartitionRecord{partition})
	require.ErrorContains(t, err, "invalid partition record: system id 00000001 node 2 input record is different")
}

func TestNewGenesisFromPartitionNodes_Ok(t *testing.T) {
	const id types.SystemID = 1
	partitionSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	partitionSigner2, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	pn1 := createPartitionNode(t, id, "1", partitionSigner)
	pn2 := createPartitionNode(t, id, "2", partitionSigner2)
	rootChainSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	rootChainVerifier, err := rootChainSigner.Verifier()
	require.NoError(t, err)
	rootPubKeyBytes, err := rootChainVerifier.MarshalPublicKey()
	require.NoError(t, err)
	pr, err := NewPartitionRecordFromNodes([]*genesis.PartitionNode{pn1, pn2})
	require.NoError(t, err)
	rg, pgs, err := NewRootGenesis("test", rootChainSigner, rootPubKeyBytes, pr)
	require.NoError(t, err)
	require.NotNil(t, rg)
	require.Equal(t, 2, len(rg.Partitions[0].Nodes))
	require.Equal(t, 1, len(pgs))
}

func TestNewGenesisForMultiplePartitions_Ok(t *testing.T) {
	const systemIdentifier1 types.SystemID = 2
	const systemIdentifier2 types.SystemID = 1
	const systemIdentifier3 types.SystemID = 0xFFFFFFFF

	partitionSigner, _ := testsig.CreateSignerAndVerifier(t)
	partitionSigner2, _ := testsig.CreateSignerAndVerifier(t)
	partitionSigner3, _ := testsig.CreateSignerAndVerifier(t)

	pn1 := createPartitionNode(t, systemIdentifier1, "1", partitionSigner)
	pn2 := createPartitionNode(t, systemIdentifier2, "2", partitionSigner2)
	pn3 := createPartitionNode(t, systemIdentifier3, "3", partitionSigner3)
	rootChainSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	rootChainVerifier, err := rootChainSigner.Verifier()
	require.NoError(t, err)
	rootPubKeyBytes, err := rootChainVerifier.MarshalPublicKey()
	require.NoError(t, err)
	pr, err := NewPartitionRecordFromNodes([]*genesis.PartitionNode{pn1, pn2, pn3})
	require.NoError(t, err)
	rg, pgs, err := NewRootGenesis("test", rootChainSigner, rootPubKeyBytes, pr)
	require.NoError(t, err)
	require.NotNil(t, rg)
	require.Equal(t, 1, len(rg.Partitions[0].Nodes))
	require.Equal(t, 3, len(pgs))
	tb, err := rg.GenerateTrustBase()
	require.NoError(t, err)
	for _, partitionGenesis := range pgs {
		require.NoError(t, partitionGenesis.IsValid(tb, gocrypto.SHA256))
	}
}

func TestNewGenesis_AddSignature(t *testing.T) {
	const id types.SystemID = 1
	partitionSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	partition := createPartition(t, id, "1", partitionSigner)
	require.NoError(t, err)
	rootChainSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	require.NoError(t, err)

	_, verifier := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	rg, ps, err := NewRootGenesis(
		"test1",
		rootChainSigner,
		rootPubKeyBytes,
		[]*genesis.PartitionRecord{partition},
		WithTotalNodes(2),
	)
	require.NoError(t, err)
	require.Equal(t, uint32(2), rg.Root.Consensus.TotalRootValidators)
	require.NoError(t, err)
	require.NotNil(t, rg)
	require.NotNil(t, ps)
	require.Equal(t, 1, len(ps))
	require.NoError(t, rg.IsValid())
	// not yet signed by all root validators
	require.Error(t, rg.Verify())
	// try to sign again using the same key and id
	rg2, err := RootGenesisAddSignature(rg, "test1", rootChainSigner, rootPubKeyBytes)
	require.ErrorContains(t, err, "genesis is already signed by node id test1")
	require.Nil(t, rg2)

	rootChainSigner2, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, verifier2 := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes2, err := verifier2.MarshalPublicKey()
	require.NoError(t, err)
	rg, err = RootGenesisAddSignature(rg, "test2", rootChainSigner2, rootPubKeyBytes2)
	require.NoError(t, err)
	require.NotNil(t, rg)
	// validate
	require.Len(t, rg.Root.RootValidators, 2)
	require.Len(t, rg.Root.Consensus.Signatures, 2)
	require.Len(t, rg.Partitions[0].Certificate.UnicitySeal.Signatures, 2)
	require.NoError(t, rg.IsValid())
	// signed by total number of root validators
	require.NoError(t, rg.Verify())
	// Test not possible to add another signature - total nodes is 3
	rootChainSigner3, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, verifier3 := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes3, err := verifier3.MarshalPublicKey()
	require.NoError(t, err)
	rg, err = RootGenesisAddSignature(rg, "test3", rootChainSigner3, rootPubKeyBytes3)
	require.ErrorContains(t, err, "genesis is already signed by maximum number of root nodes")
	require.Nil(t, rg)
}

func TestNewGenesis_MergeGenesisFiles(t *testing.T) {
	const id types.SystemID = 1
	partitionSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	partition := createPartition(t, id, "1", partitionSigner)
	require.NoError(t, err)
	const totalRootNodes = 2
	s1, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, v1 := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := v1.MarshalPublicKey()
	require.NoError(t, err)
	// generate root genesis 1
	rg1, _, err := NewRootGenesis("test1",
		s1,
		rootPubKeyBytes,
		[]*genesis.PartitionRecord{partition},
		WithTotalNodes(totalRootNodes),
		WithBlockRate(genesis.MinBlockRateMs),
		WithConsensusTimeout(genesis.MinBlockRateMs+genesis.MinConsensusTimeout))
	require.NoError(t, err)
	require.NoError(t, rg1.IsValid())
	// generate genesis 2
	s2, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, v2 := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes2, err := v2.MarshalPublicKey()
	require.NoError(t, err)
	rg2, _, err := NewRootGenesis("test2",
		s2,
		rootPubKeyBytes2,
		[]*genesis.PartitionRecord{partition},
		WithTotalNodes(totalRootNodes),
		WithBlockRate(genesis.MinBlockRateMs),
		WithConsensusTimeout(genesis.MinBlockRateMs+genesis.MinConsensusTimeout))
	require.NoError(t, err)
	require.NoError(t, rg2.IsValid())
	geneses := []*genesis.RootGenesis{rg1, rg2}
	// merge genesis files
	rootGenesis, partitionGenesis, err := MergeRootGenesisFiles(geneses)
	require.NoError(t, err)
	require.NotNil(t, rootGenesis)
	require.NotNil(t, partitionGenesis)
	require.NoError(t, rootGenesis.IsValid())
	require.NoError(t, rootGenesis.Verify())
}

func TestGenerateTrustBase_CustomQuorumThreshold(t *testing.T) {
	rg := createRootGenesis(t)
	tb, err := rg.GenerateTrustBase(types.WithQuorumThreshold(2))
	require.NoError(t, err)
	for _, partitionGenesis := range rg.Partitions {
		require.ErrorContains(t, partitionGenesis.IsValid(tb, gocrypto.SHA256), "quorum not reached")
	}
	tb, err = rg.GenerateTrustBase(types.WithQuorumThreshold(1))
	require.NoError(t, err)
	for _, partitionGenesis := range rg.Partitions {
		require.NoError(t, partitionGenesis.IsValid(tb, gocrypto.SHA256))
	}
}

func createRootGenesis(t *testing.T) *genesis.RootGenesis {
	partitionSigner, _ := testsig.CreateSignerAndVerifier(t)
	partitionNode := createPartitionNode(t, 1, "1", partitionSigner)
	rootChainSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	rootChainVerifier, err := rootChainSigner.Verifier()
	require.NoError(t, err)
	rootPubKeyBytes, err := rootChainVerifier.MarshalPublicKey()
	require.NoError(t, err)
	pr, err := NewPartitionRecordFromNodes([]*genesis.PartitionNode{partitionNode})
	require.NoError(t, err)
	rg, rgs, err := NewRootGenesis("test", rootChainSigner, rootPubKeyBytes, pr)
	require.NoError(t, err)
	require.NotNil(t, rg)
	require.Len(t, rgs, 1)
	return rg
}
