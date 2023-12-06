package testutils

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/testutils"
	"github.com/alphabill-org/alphabill/testutils/network"
	testpeer "github.com/alphabill-org/alphabill/testutils/peer"
	"github.com/alphabill-org/alphabill/testutils/sig"
	"github.com/alphabill-org/alphabill/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type TestNode struct {
	Signer   crypto.Signer
	Verifier crypto.Verifier
	PeerConf *network.PeerConfiguration
}

func NewTestNode(t *testing.T) *TestNode {
	t.Helper()
	node := &TestNode{PeerConf: testpeer.CreatePeerConfiguration(t)}
	node.Signer, node.Verifier = testsig.CreateSignerAndVerifier(t)
	return node
}

func CreatePartitionNodesAndPartitionRecord(t *testing.T, ir *types.InputRecord, systemID types.SystemID32, nrOfValidators int) (partitionNodes []*TestNode, record *genesis.PartitionRecord) {
	t.Helper()
	record = &genesis.PartitionRecord{
		SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
			SystemIdentifier: systemID.ToSystemID(),
			T2Timeout:        2500,
		},
		Validators: []*genesis.PartitionNode{},
	}
	for i := 0; i < nrOfValidators; i++ {
		partitionNode := NewTestNode(t)

		rawEncPubKey := partitionNode.PeerConf.KeyPair.PublicKey

		rawSigningPubKey, err := partitionNode.Verifier.MarshalPublicKey()
		require.NoError(t, err)

		req := &certification.BlockCertificationRequest{
			SystemIdentifier: systemID.ToSystemID(),
			NodeIdentifier:   partitionNode.PeerConf.ID.String(),
			InputRecord:      ir,
		}
		err = req.Sign(partitionNode.Signer)
		require.NoError(t, err)

		record.Validators = append(record.Validators, &genesis.PartitionNode{
			NodeIdentifier:            partitionNode.PeerConf.ID.String(),
			SigningPublicKey:          rawSigningPubKey,
			EncryptionPublicKey:       rawEncPubKey,
			BlockCertificationRequest: req,
		})

		partitionNodes = append(partitionNodes, partitionNode)
	}
	return partitionNodes, record
}

func CreateBlockCertificationRequest(t *testing.T, ir *types.InputRecord, sysID types.SystemID32, node *TestNode) *certification.BlockCertificationRequest {
	t.Helper()
	r1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysID.ToSystemID(),
		NodeIdentifier:   node.PeerConf.ID.String(),
		InputRecord:      ir,
		RootRoundNumber:  1,
	}
	require.NoError(t, r1.Sign(node.Signer))
	return r1
}

func MockValidatorNetReceives(t *testing.T, net *testnetwork.MockNet, id peer.ID, msgType string, msg any) {
	t.Helper()
	net.Receive(msg)
	// wait for message to be consumed
	require.Eventually(t, func() bool { return len(net.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
}

func MockAwaitMessage[T any](t *testing.T, net *testnetwork.MockNet, msgType string) T {
	t.Helper()
	var msg any
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

func MockNetAwaitMultiple[T any](t *testing.T, net *testnetwork.MockNet, msgType string, nof int) []T {
	t.Helper()
	result := make([]T, nof)
	require.Eventually(t, func() bool {
		messages := net.SentMessages(msgType)
		if len(messages) >= nof {
			for i, msg := range messages {
				result[i] = msg.Message.(T)
			}
			return true
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	// cleat the queue
	net.ResetSentMessages(msgType)
	return result
}
