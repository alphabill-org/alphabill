package partition

import (
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"
	testnetwork "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/network"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peerstore"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
)

func TestNewBlockProposalSubscriber_NotOk(t *testing.T) {
	type args struct {
		self       *network.Peer
		rootNodeID peer.ID
		eb         *eventbus.EventBus
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "peer is nil",
			args: args{
				self:       nil,
				rootNodeID: "1",
				eb:         eventbus.New(),
			},
			wantErr: ErrPeerIsNil,
		},
		{
			name: "eventbus is nil",
			args: args{
				self:       createPeer(t),
				rootNodeID: "1",
				eb:         nil,
			},
			wantErr: ErrEventBusIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewBlockProposalSubscriber(tt.args.self, capacity, time.Second, tt.args.eb)
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

type testBlockProposalHandler struct {
	requestReceived bool
}

func (h *testBlockProposalHandler) handle(s libp2pNetwork.Stream) {
	h.requestReceived = true
}

func TestSendProposal_Ok(t *testing.T) {
	peer1 := testnetwork.CreatePeer(t)
	peer2 := testnetwork.CreatePeer(t)
	blockProposalHandler := &testBlockProposalHandler{}
	peer2.RegisterProtocolHandler(blockproposal.ProtocolBlockProposal, blockProposalHandler.handle)
	addPersistentPeers(t, peer2, peer1)

	eb := eventbus.New()
	sub, err := NewBlockProposalSubscriber(peer1, capacity, time.Second, eb)
	require.NoError(t, err)
	defer sub.Close()

	err = eb.Submit(eventbus.TopicBlockProposalOutput, eventbus.BlockProposalEvent{BlockProposal: &blockproposal.BlockProposal{}})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return blockProposalHandler.requestReceived
	}, test.WaitDuration, test.WaitTick)
}

func TestReceiveProposal_Ok(t *testing.T) {
	peer1 := testnetwork.CreatePeer(t)
	peer2 := testnetwork.CreatePeer(t)

	addPersistentPeers(t, peer2, peer1)

	eb := eventbus.New()
	sub, err := NewBlockProposalSubscriber(peer1, capacity, time.Second, eb)
	require.NoError(t, err)
	defer sub.Close()

	eb2 := eventbus.New()
	sub2, err := NewBlockProposalSubscriber(peer2, capacity, time.Second, eb2)
	require.NoError(t, err)
	defer sub2.Close()

	ch, err := eb2.Subscribe(eventbus.TopicBlockProposalInput, 1)
	require.NoError(t, err)

	err = eb.Submit(eventbus.TopicBlockProposalOutput, eventbus.BlockProposalEvent{BlockProposal: &blockproposal.BlockProposal{}})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		e := <-ch
		return e != nil
	}, test.WaitDuration, test.WaitTick)
}

func addPersistentPeers(t *testing.T, peer2 *network.Peer, peer1 *network.Peer) {
	peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
	pubKey, err := peer2.PublicKey()
	require.NoError(t, err)
	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	require.NoError(t, err)

	peer1.Configuration().PersistentPeers = append(peer1.Configuration().PersistentPeers, &network.PeerInfo{
		Address:   fmt.Sprintf("%v", peer2.MultiAddresses()),
		PublicKey: pubKeyBytes,
	})
}
