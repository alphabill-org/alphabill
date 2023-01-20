package network

import (
	"testing"
	"time"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestNewLibP2PNetwork_Ok(t *testing.T) {
	net, err := NewLibP2PNetwork(createPeer(t), 10)
	require.NoError(t, err)
	defer net.Close()
	require.Equal(t, cap(net.ReceivedChannel()), 10)
	require.Equal(t, 0, len(net.sendProtocols))
	require.Equal(t, 0, len(net.receiveProtocols))
}

func TestNewLibP2PNetwork_PeerIsNil(t *testing.T) {
	net, err := NewLibP2PNetwork(nil, 10)
	require.ErrorContains(t, err, ErrStrPeerIsNil)
	require.Nil(t, net)
}

func TestNewValidatorLibP2PNetwork_Ok(t *testing.T) {
	net, err := NewLibP2PValidatorNetwork(createPeer(t), DefaultValidatorNetOptions)
	require.NoError(t, err)
	defer net.Close()
	require.Equal(t, cap(net.ReceivedChannel()), 1000)
	require.Equal(t, 6, len(net.sendProtocols))
	require.Equal(t, 5, len(net.receiveProtocols))
}

func TestNetworkSendFunctionReturnsNilIfNoPeers(t *testing.T) {
	net, err := NewLibP2PValidatorNetwork(createPeer(t), DefaultValidatorNetOptions)
	require.NoError(t, err)
	defer net.Close()

	err = net.Send(OutputMessage{
		Protocol: ProtocolBlockProposal,
		Message:  nil,
	}, []peer.ID{})
	require.NoError(t, err)
}

func TestNewRootNodeLibP2PNetwork_Ok(t *testing.T) {
	net, err := NewLibP2PRootChainNetwork(createPeer(t), 1000, time.Second)
	require.NoError(t, err)
	defer net.Close()
	require.Equal(t, cap(net.ReceivedChannel()), 1000)
	require.Equal(t, 1, len(net.sendProtocols))
	require.Equal(t, 2, len(net.receiveProtocols))
}

func TestNewRootNodeLibP2PNetwork_SendToSelf(t *testing.T) {
	self := createPeer(t)
	net, err := NewLibP2RootConsensusNetwork(self, 1000, time.Second)
	require.NoError(t, err)
	defer net.Close()
	require.Equal(t, cap(net.ReceivedChannel()), 1000)
	require.Equal(t, 6, len(net.sendProtocols))
	require.Equal(t, 6, len(net.receiveProtocols))

	err = net.Send(OutputMessage{
		Protocol: ProtocolRootVote,
		Message:  nil,
	}, []peer.ID{self.ID()})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		m := <-net.ReceivedChannel()
		require.Equal(t, ProtocolRootVote, m.Protocol)
		require.True(t, proto.Equal(nil, m.Message))
		return true
	}, test.WaitDuration, test.WaitTick)
}
