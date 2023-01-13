package testutils

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testpeer "github.com/alphabill-org/alphabill/internal/testutils/peer"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type TestNode struct {
	Signer   crypto.Signer
	Verifier crypto.Verifier
	Peer     *network.Peer
}

func NewTestNode(t *testing.T) *TestNode {
	t.Helper()
	node := &TestNode{Peer: testpeer.CreatePeer(t)}
	node.Signer, node.Verifier = testsig.CreateSignerAndVerifier(t)
	return node
}

func CreatePartitionNodesAndPartitionRecord(t *testing.T, ir *certificates.InputRecord, systemID []byte, nrOfValidators int) (partitionNodes []*TestNode, record *genesis.PartitionRecord) {
	record = &genesis.PartitionRecord{
		SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
			SystemIdentifier: systemID,
			T2Timeout:        2500,
		},
		Validators: []*genesis.PartitionNode{},
	}
	for i := 0; i < nrOfValidators; i++ {
		partitionNode := NewTestNode(t)

		encPubKey, err := partitionNode.Peer.PublicKey()
		require.NoError(t, err)
		rawEncPubKey, err := encPubKey.Raw()
		require.NoError(t, err)

		rawSigningPubKey, err := partitionNode.Verifier.MarshalPublicKey()
		require.NoError(t, err)

		req := &certification.BlockCertificationRequest{
			SystemIdentifier: systemID,
			NodeIdentifier:   partitionNode.Peer.ID().String(),
			RootRoundNumber:  1,
			InputRecord:      ir,
		}
		err = req.Sign(partitionNode.Signer)
		require.NoError(t, err)

		record.Validators = append(record.Validators, &genesis.PartitionNode{
			NodeIdentifier:            partitionNode.Peer.ID().String(),
			SigningPublicKey:          rawSigningPubKey,
			EncryptionPublicKey:       rawEncPubKey,
			BlockCertificationRequest: req,
		})

		partitionNodes = append(partitionNodes, partitionNode)
	}
	return partitionNodes, record
}

func CreateBlockCertificationRequest(t *testing.T, pn *genesis.PartitionNode, id []byte, newHash []byte, blockHash []byte, partitionNode *TestNode) *certification.BlockCertificationRequest {
	t.Helper()
	r1 := &certification.BlockCertificationRequest{
		SystemIdentifier: id,
		NodeIdentifier:   pn.BlockCertificationRequest.NodeIdentifier,
		RootRoundNumber:  1,
		InputRecord: &certificates.InputRecord{
			PreviousHash: pn.BlockCertificationRequest.InputRecord.Hash,
			Hash:         newHash,
			BlockHash:    blockHash,
			SummaryValue: pn.BlockCertificationRequest.InputRecord.SummaryValue,
		},
	}
	require.NoError(t, r1.Sign(partitionNode.Signer))
	return r1
}

func MockValidatorNetReceives(t *testing.T, net *testnetwork.MockNet, id peer.ID, msgType string, msg proto.Message) {
	net.Receive(network.ReceivedMessage{
		From:     id,
		Protocol: msgType,
		Message:  msg,
	})
	// wait for message to be consumed
	require.Eventually(t, func() bool { return len(net.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
}

func MockAwaitMessage[T any](t *testing.T, net *testnetwork.MockNet, msgType string) T {
	var msg proto.Message
	require.Eventually(t, func() bool {
		messages := net.SentMessages(msgType)
		if len(messages) > 0 {
			msg = messages[0].Message
			return true
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	// cleat the queue
	net.ResetSentMessages(msgType)
	return msg.(T)
}
