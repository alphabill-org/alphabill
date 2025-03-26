package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testpeer "github.com/alphabill-org/alphabill/internal/testutils/peer"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
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

func (n *TestNode) NodeInfo(t *testing.T) *types.NodeInfo {
	sigKey, err := n.Verifier.MarshalPublicKey()
	require.NoError(t, err)
	return &types.NodeInfo{
		NodeID: n.PeerConf.ID.String(),
		SigKey: sigKey,
		Stake:  1,
	}
}

func CreateTestNodes(t *testing.T, nrOfValidators int) ([]*TestNode, []*types.NodeInfo) {
	t.Helper()
	var testNodes []*TestNode
	var nodeInfos []*types.NodeInfo
	for i := 0; i < nrOfValidators; i++ {
		tn := NewTestNode(t)
		testNodes = append(testNodes, tn)
		nodeInfos = append(nodeInfos, tn.NodeInfo(t))
	}
	return testNodes, nodeInfos
}

func CreateBlockCertificationRequest(t *testing.T, ir *types.InputRecord, partitionID types.PartitionID, node *TestNode) *certification.BlockCertificationRequest {
	t.Helper()
	r1 := &certification.BlockCertificationRequest{
		PartitionID: partitionID,
		NodeID:      node.PeerConf.ID.String(),
		InputRecord: ir,
	}
	require.NoError(t, r1.Sign(node.Signer))
	return r1
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
	}, test.WaitDuration*3, test.WaitShortTick)
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
			for i := range result {
				result[i] = messages[i].Message.(T)
			}
			return true
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	// cleat the queue
	net.ResetSentMessages(msgType)
	return result
}

// RandomNodeID returns base58 node id and the corresponding auth public key
func RandomNodeID(t *testing.T) (string, []byte) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	publicKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	libp2pPublicKey, err := libp2pcrypto.UnmarshalSecp256k1PublicKey(publicKey)
	require.NoError(t, err)
	nodeID, err := peer.IDFromPublicKey(libp2pPublicKey)
	require.NoError(t, err)
	return nodeID.String(), publicKey
}
