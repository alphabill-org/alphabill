package rootchain

import (
	"strings"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
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
	rg, ps, err := NewGenesis([]*genesis.PartitionRecord{partition}, rootChainSigner)
	require.NoError(t, err)
	require.NotNil(t, rg)
	require.NotNil(t, ps)
	require.Equal(t, 1, len(ps))
	require.NoError(t, rg.IsValid(rootChainVerifier))
	WriteDebugJsonLog(logger, "", rg)

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
		NodeIdentifier: "2",
		PublicKey:      pubKey,
		P1Request:      req,
	}
	partition.Validators = append(partition.Validators, pr)

	require.NoError(t, err)
	rootChainSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	_, _, err = NewGenesis([]*genesis.PartitionRecord{partition}, rootChainSigner)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "has not reached a consensus"))
}

func createPartition(t *testing.T, systemIdentifier []byte, nodeID string, partitionSigner *crypto.InMemorySecp256K1Signer) *genesis.PartitionRecord {
	p1Req := createInputRequest(t, systemIdentifier, nodeID, partitionSigner)
	pubKey, _, err := GetPublicKeyAndVerifier(partitionSigner)
	require.NoError(t, err)

	return &genesis.PartitionRecord{
		SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
			SystemIdentifier: systemIdentifier,
			T2Timeout:        2500,
		},
		Validators: []*genesis.PartitionNode{{
			NodeIdentifier: nodeID,
			PublicKey:      pubKey,
			P1Request:      p1Req,
		}},
	}
}

func createInputRequest(t *testing.T, systemIdentifier []byte, nodeID string, partitionSigner *crypto.InMemorySecp256K1Signer) *p1.P1Request {
	p1Req := &p1.P1Request{
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

	err := p1Req.Sign(partitionSigner)
	require.NoError(t, err)
	return p1Req
}
