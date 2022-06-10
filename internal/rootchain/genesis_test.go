package rootchain

import (
	"strings"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network/protocol/certification"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network/protocol/genesis"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestNewGenesis_Ok(t *testing.T) {
	id := []byte{0, 0, 0, 1}
	partitionSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	partition := createPartition(t, id, "1", partitionSigner)
	require.NoError(t, err)
	rootChainSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	rootChainVerifier, err := rootChainSigner.Verifier()
	require.NoError(t, err)

	_, encPubKey := testsig.CreateSignerAndVerifier(t)

	rg, ps, err := NewGenesis([]*genesis.PartitionRecord{partition}, rootChainSigner, encPubKey)
	require.NoError(t, err)
	require.NotNil(t, rg)
	require.NotNil(t, ps)
	require.Equal(t, 1, len(ps))
	require.NoError(t, rg.IsValid(rootChainVerifier))
}

func TestNewGenesis_ConsensusNotPossible(t *testing.T) {
	id := []byte{0, 0, 0, 1}
	partitionSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	partitionSigner2, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	partition := createPartition(t, id, "1", partitionSigner)

	req := createInputRequest(t, id, "2", partitionSigner2)
	req.InputRecord.Hash = []byte{1, 1, 1, 1}
	require.NoError(t, req.Sign(partitionSigner2))
	pubKey, _, err := GetPublicKeyAndVerifier(partitionSigner2)
	require.NoError(t, err)
	pr := &genesis.PartitionNode{
		NodeIdentifier:            "2",
		SigningPublicKey:          pubKey,
		EncryptionPublicKey:       pubKey,
		BlockCertificationRequest: req,
	}
	partition.Validators = append(partition.Validators, pr)

	require.NoError(t, err)
	rootChainSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	_, _, err = NewGenesis([]*genesis.PartitionRecord{partition}, rootChainSigner, encPubKey)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "has not reached a consensus"))
}

func TestNewGenesisFromPartitionNodes_Ok(t *testing.T) {
	id := []byte{0, 0, 0, 1}
	partitionSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	partitionSigner2, err := crypto.NewInMemorySecp256K1Signer()

	pn1 := createPartitionNode(t, id, "1", partitionSigner)
	pn2 := createPartitionNode(t, id, "2", partitionSigner2)
	rootChainSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	rootChainVerifier, err := rootChainSigner.Verifier()
	require.NoError(t, err)
	rg, pgs, err := NewGenesisFromPartitionNodes([]*genesis.PartitionNode{pn1, pn2}, rootChainSigner, rootChainVerifier)
	require.NoError(t, err)
	require.NotNil(t, rg)
	require.Equal(t, 2, len(rg.Partitions[0].Nodes))
	require.Equal(t, 1, len(pgs))
}

func createPartition(t *testing.T, systemIdentifier []byte, nodeID string, partitionSigner *crypto.InMemorySecp256K1Signer) *genesis.PartitionRecord {
	req := createInputRequest(t, systemIdentifier, nodeID, partitionSigner)
	pubKey, _, err := GetPublicKeyAndVerifier(partitionSigner)
	require.NoError(t, err)

	return &genesis.PartitionRecord{
		SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
			SystemIdentifier: systemIdentifier,
			T2Timeout:        2500,
		},
		Validators: []*genesis.PartitionNode{{
			NodeIdentifier:            nodeID,
			SigningPublicKey:          pubKey,
			EncryptionPublicKey:       pubKey,
			BlockCertificationRequest: req,
		}},
	}
}

func createPartitionNode(t *testing.T, systemIdentifier []byte, nodeID string, partitionSigner *crypto.InMemorySecp256K1Signer) *genesis.PartitionNode {
	req := createInputRequest(t, systemIdentifier, nodeID, partitionSigner)
	pubKey, _, err := GetPublicKeyAndVerifier(partitionSigner)
	require.NoError(t, err)

	return &genesis.PartitionNode{
		NodeIdentifier:            nodeID,
		SigningPublicKey:          pubKey,
		EncryptionPublicKey:       pubKey,
		BlockCertificationRequest: req,
		T2Timeout:                 2500,
	}
}

func createInputRequest(t *testing.T, systemIdentifier []byte, nodeID string, partitionSigner *crypto.InMemorySecp256K1Signer) *certification.BlockCertificationRequest {
	req := &certification.BlockCertificationRequest{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   nodeID,
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    make([]byte, 32),
			SummaryValue: []byte{1, 0, 0},
		},
	}

	err := req.Sign(partitionSigner)
	require.NoError(t, err)
	return req
}
