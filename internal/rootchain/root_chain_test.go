package rootchain

import (
	"bytes"
	"testing"
	"time"

	certificates2 "gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/certification"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testnetwork "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/network"
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
)

var partitionID = []byte{0, 0xFF, 0, 1}
var partitionInputRecord = &certificates.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 0, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
}

type node struct {
	signingKey       crypto.Signer
	signingPublicKey crypto.Verifier
	peer             *network.Peer
}

func TestNewRootChain_Ok(t *testing.T) {
	rc, peer, signer := initRootChain(t)
	defer rc.Close()
	require.NotNil(t, signer)
	require.NotNil(t, rc)
	require.NotNil(t, peer)
}

func TestNewRootChain_LoadUsingOptions(t *testing.T) {
	duration := 2000 * time.Second
	rc, peer, signer := initRootChain(t, WithRequestChCapacity(10), WithT3Timeout(duration))
	defer rc.Close()
	require.NotNil(t, signer)
	require.NotNil(t, rc)
	require.NotNil(t, peer)
}

func TestPartitionReceivesUnicityCertificates(t *testing.T) {
	partitionNodes, partitionRecord := createPartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 3)
	rootNode := createNode(t)
	verifier, err := crypto.NewVerifierSecp256k1(rootNode.GetEncryptionPublicKeyBytes(t))
	require.NoError(t, err)

	rootGenesis, _, err := NewGenesis([]*genesis.PartitionRecord{partitionRecord}, rootNode.signingKey, verifier)
	require.NoError(t, err)

	mockNet := testnetwork.NewMockNetwork()
	rc, err := NewRootChain(rootNode.peer, rootGenesis, rootNode.signingKey, mockNet)
	defer rc.Close()
	require.NoError(t, err)

	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	mockNet.Receive(network.ReceivedMessage{
		From:     partitionNodes[0].peer.ID(),
		Protocol: certification.ProtocolBlockCertification,
		Message:  createBlockCertificationRequest(t, partitionRecord.Validators[0], newHash, blockHash, partitionNodes[0]),
	})

	mockNet.Receive(network.ReceivedMessage{
		From:     partitionNodes[1].peer.ID(),
		Protocol: certification.ProtocolBlockCertification,
		Message:  createBlockCertificationRequest(t, partitionRecord.Validators[1], newHash, blockHash, partitionNodes[1]),
	})
	require.Eventually(t, func() bool {
		messages := mockNet.SentMessages[certificates2.ProtocolReceiveUnicityCertificate]
		if len(messages) > 0 {
			m := messages[0]
			uc := m.Message.(*certificates.UnicityCertificate)
			return bytes.Equal(uc.InputRecord.Hash, newHash) && bytes.Equal(uc.InputRecord.BlockHash, blockHash)
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func createBlockCertificationRequest(t *testing.T, pn *genesis.PartitionNode, newHash []byte, blockHash []byte, partitionNode *node) *certification.BlockCertificationRequest {
	t.Helper()
	r1 := &certification.BlockCertificationRequest{
		SystemIdentifier: partitionID,
		NodeIdentifier:   pn.BlockCertificationRequest.NodeIdentifier,
		RootRoundNumber:  1,
		InputRecord: &certificates.InputRecord{
			PreviousHash: pn.BlockCertificationRequest.InputRecord.Hash,
			Hash:         newHash,
			BlockHash:    blockHash,
			SummaryValue: pn.BlockCertificationRequest.InputRecord.SummaryValue,
		},
	}
	require.NoError(t, r1.Sign(partitionNode.signingKey))
	return r1
}

func (n *node) GetEncryptionPublicKeyBytes(t *testing.T) []byte {
	pk, err := n.peer.PublicKey()
	require.NoError(t, err)
	bytes, err := pk.Raw()
	require.NoError(t, err)
	return bytes
}

func createPartitionNodesAndPartitionRecord(t *testing.T, ir *certificates.InputRecord, systemID []byte, nrOfValidators int) (partitionNodes []*node, record *genesis.PartitionRecord) {
	record = &genesis.PartitionRecord{
		SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
			SystemIdentifier: systemID,
			T2Timeout:        2500,
		},
		Validators: []*genesis.PartitionNode{},
	}
	for i := 0; i < nrOfValidators; i++ {
		partitionNode := createNode(t)

		encPubKey, err := partitionNode.peer.PublicKey()
		require.NoError(t, err)
		rawEncPubKey, err := encPubKey.Raw()
		require.NoError(t, err)

		rawSigningPubKey, err := partitionNode.signingPublicKey.MarshalPublicKey()
		require.NoError(t, err)

		req := &certification.BlockCertificationRequest{
			SystemIdentifier: systemID,
			NodeIdentifier:   peer.Encode(partitionNode.peer.ID()),
			RootRoundNumber:  1,
			InputRecord:      ir,
		}
		err = req.Sign(partitionNode.signingKey)
		require.NoError(t, err)

		record.Validators = append(record.Validators, &genesis.PartitionNode{
			NodeIdentifier:            peer.Encode(partitionNode.peer.ID()),
			SigningPublicKey:          rawSigningPubKey,
			EncryptionPublicKey:       rawEncPubKey,
			BlockCertificationRequest: req,
		})

		partitionNodes = append(partitionNodes, partitionNode)
	}
	return partitionNodes, record
}

func createNode(t *testing.T) *node {
	t.Helper()
	partitionNode := &node{peer: testnetwork.CreatePeer(t)}
	partitionNode.signingKey, partitionNode.signingPublicKey = testsig.CreateSignerAndVerifier(t)
	return partitionNode
}

func initRootChain(t *testing.T, opts ...Option) (*RootChain, *network.Peer, crypto.Signer) {
	t.Helper()
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	peer := testnetwork.CreatePeer(t)
	_, _, partition, _ := createPartitionRecord(t, partitionInputRecord, partitionID, 3)
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	rootGenesis, _, err := NewGenesis([]*genesis.PartitionRecord{partition}, rootSigner, encPubKey)
	require.NoError(t, err)
	rc, err := NewRootChain(peer, rootGenesis, rootSigner, testnetwork.NewMockNetwork(), opts...)
	require.NoError(t, err)
	return rc, peer, rootSigner
}
