package rootchain

import (
	"context"
	gocrypto "crypto"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/testnetwork"
)

var partitionID = []byte{0, 0xFF, 0, 1}
var partitionInputRecord = &certificates.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 0, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
}

func TestNewRootChain_Ok(t *testing.T) {
	rc, peer, signer := initRootChain(t)
	defer rc.Close()
	require.NotNil(t, signer)
	require.NotNil(t, rc)
	require.NotNil(t, peer)
}

func TestNewRootChain_LoadUsingOptions(t *testing.T) {
	rc, peer, signer := initRootChain(t, WithRequestChCapacity(10), WithT3Timeout(2000*time.Second))
	defer rc.Close()
	require.NotNil(t, signer)
	require.NotNil(t, rc)
	require.NotNil(t, peer)
}

func TestRootChain_SendMultipleRequests(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	rootVerifier, err := rootSigner.Verifier()
	require.NoError(t, err)

	rootPeer := testnetwork.CreatePeer(t)
	partitionsSigners, _, partition := createPartitionRecord(t, partitionInputRecord, partitionID, 1)
	rootGenesis, pgs, err := NewGenesis([]*genesis.PartitionRecord{partition}, rootSigner)
	require.NoError(t, err)
	rc, err := NewRootChain(rootPeer, rootGenesis, rootSigner)
	defer rc.Close()
	require.NoError(t, err)
	clientPeer := testnetwork.CreatePeer(t)

	clientConf := p1.ClientConfiguration{
		Signer:                      partitionsSigners[0],
		NodeIdentifier:              "0",
		SystemIdentifier:            pgs[0].SystemDescriptionRecord.SystemIdentifier,
		SystemDescriptionRecordHash: pgs[0].SystemDescriptionRecord.Hash(gocrypto.SHA256),
		HashAlgorithm:               gocrypto.SHA256,
		RootChainRoundNumber:        pgs[0].Certificate.UnicitySeal.RootChainRoundNumber,
		PreviousHash:                pgs[0].Certificate.InputRecord.Hash,
	}

	serverConf := p1.ServerConfiguration{
		RootChainID:     rootPeer.ID(),
		ServerAddresses: rootPeer.MultiAddresses(),
		RootVerifier:    rootVerifier,
	}

	client, err := p1.NewClient(clientPeer, clientConf, serverConf)
	require.NoError(t, err)

	response, err := client.SendSync(context.Background(), []byte{0, 0, 0, 2}, []byte{0, 0, 1, 1}, []byte{0, 0, 1, 3})
	require.NoError(t, err)
	uc1 := response.Message
	require.NoError(t, uc1.IsValid(rootVerifier, gocrypto.SHA256, partitionID, partition.SystemDescriptionRecord.Hash(gocrypto.SHA256)))

	response2, err := client.SendSync(context.Background(), []byte{0, 0, 0, uint8(5)}, []byte{0, 0, 1, uint8(5)}, []byte{0, 0, 1, 3})
	uc2 := response2.Message
	require.NoError(t, err)
	require.NoError(t, uc2.IsValid(rootVerifier, gocrypto.SHA256, partitionID, partition.SystemDescriptionRecord.Hash(gocrypto.SHA256)))

	require.Equal(t, uc1.UnicitySeal.Hash, uc2.UnicitySeal.PreviousHash)
	require.Equal(t, uc1.UnicitySeal.RootChainRoundNumber+1, uc2.UnicitySeal.RootChainRoundNumber)
}

func initRootChain(t *testing.T, opts ...Option) (*RootChain, *network.Peer, crypto.Signer) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	peer := testnetwork.CreatePeer(t)
	_, _, partition := createPartitionRecord(t, partitionInputRecord, partitionID, 3)
	rootGenesis, _, err := NewGenesis([]*genesis.PartitionRecord{partition}, rootSigner)
	require.NoError(t, err)
	rc, err := NewRootChain(peer, rootGenesis, rootSigner, opts...)
	require.NoError(t, err)
	return rc, peer, rootSigner
}
