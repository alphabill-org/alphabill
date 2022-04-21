package blockproposal

import (
	"fmt"
	"strings"
	"testing"
	"time"

	golog "github.com/ipfs/go-log"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"google.golang.org/protobuf/proto"

	"github.com/libp2p/go-libp2p-core/peerstore"

	"github.com/libp2p/go-libp2p-core/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/testnetwork"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
)

const defaultTimeout = 300 * time.Millisecond

var emptyProposal = &BlockProposal{
	SystemIdentifier:   nil,
	NodeIdentifier:     "",
	UnicityCertificate: nil,
	Transactions:       nil,
	Signature:          nil,
}

type pcs1oRequest struct {
	req   *BlockProposal
	sleep bool
}

func (p *pcs1oRequest) r(req *BlockProposal) {
	if p.sleep {
		time.Sleep(10 * time.Second)
	}
	p.req = req
}

func init() {
	golog.SetAllLoggers(golog.LevelDebug) // change this to Debug if libp2p logs are needed
}

func TestNew(t *testing.T) {
	type args struct {
		self           *network.Peer
		timeout        time.Duration
		requestHandler ProtocolHandler
	}

	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "self is nil",
			args: args{
				self:           nil,
				timeout:        defaultTimeout,
				requestHandler: func(req *BlockProposal) {},
			},
			wantErr: ErrPeerIsNil,
		},
		{
			name: "request handler is nil",
			args: args{
				self:           testnetwork.CreatePeer(t),
				timeout:        defaultTimeout,
				requestHandler: nil,
			},
			wantErr: ErrRequestHandlerIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.self, tt.args.timeout, tt.args.requestHandler)
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

func TestSendPC1ORequest_RequestIsNil(t *testing.T) {
	pc1o, err := New(testnetwork.CreatePeer(t), defaultTimeout, func(req *BlockProposal) {})
	require.NoError(t, err)
	require.ErrorIs(t, pc1o.Publish(nil), ErrRequestIsNil)
}

func TestSendPC1ORequest_SingleNodeOk(t *testing.T) {
	leader, err := New(testnetwork.CreatePeer(t), defaultTimeout, func(req *BlockProposal) {})
	require.NoError(t, err)
	require.NoError(t, leader.Publish(emptyProposal))
}

func TestSendPC1ORequestToMultipleNodes_Ok(t *testing.T) {
	leader, err := New(testnetwork.CreatePeer(t), defaultTimeout, func(req *BlockProposal) {})
	_, reqStores, err := createNodes(t, 4, leader.self)
	require.NoError(t, err)
	require.NoError(t, leader.Publish(emptyProposal))
	for _, r := range reqStores {
		require.Eventually(t, func() bool {
			return proto.Equal(emptyProposal, r.req)
		}, test.WaitDuration, test.WaitTick)
	}
}

func TestSendPC1ORequestToMultipleNodes_FollowerRefusesConnection(t *testing.T) {
	leader, err := New(testnetwork.CreatePeer(t), defaultTimeout, func(req *BlockProposal) {})
	followers, reqStores, err := createNodes(t, 4, leader.self)

	require.NoError(t, err)
	err = followers[0].self.Close()
	require.NoError(t, err)
	err = leader.Publish(emptyProposal)
	require.True(t, strings.Contains(err.Error(), "failed to open stream"))
	for i := 1; i < 4; i++ {
		require.Eventually(t, func() bool {
			return proto.Equal(emptyProposal, reqStores[i].req)
		}, test.WaitDuration, test.WaitTick)
	}

}

func TestSendPC1ORequestToMultipleNodes_OneNodeFails(t *testing.T) {
	leader, err := New(testnetwork.CreatePeer(t), defaultTimeout, func(req *BlockProposal) {})
	_, reqStores, err := createNodes(t, 2, leader.self)
	require.NoError(t, err)

	reqStores[1].sleep = true

	require.NoError(t, leader.Publish(emptyProposal))
	require.Eventually(t, func() bool {
		return proto.Equal(emptyProposal, reqStores[0].req)
	}, 100*time.Millisecond, 10*time.Millisecond)
	require.Never(t, func() bool {
		return proto.Equal(emptyProposal, reqStores[1].req)
	}, 100*time.Millisecond, 10*time.Millisecond)

}

func createNodes(t *testing.T, nrOfNodes int, leader *network.Peer) ([]*Protocol, []*pcs1oRequest, error) {
	peers := make([]*Protocol, nrOfNodes)
	handlers := make([]*pcs1oRequest, nrOfNodes)
	leaderPeers := leader.Configuration().PersistentPeers
	for i := 0; i < nrOfNodes; i++ {
		peer := testnetwork.CreatePeer(t)
		pubKey, err := peer.PublicKey()
		require.NoError(t, err)

		pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
		require.NoError(t, err)

		leaderPeers = append(leaderPeers, &network.PeerInfo{
			Address:   fmt.Sprintf("%v", peer.MultiAddresses()),
			PublicKey: pubKeyBytes,
		})
		leader.Network().Peerstore().SetAddrs(peer.ID(), peer.MultiAddresses(), peerstore.PermanentAddrTTL)
		handlers[i] = &pcs1oRequest{}
		peers[i], err = New(peer, defaultTimeout, handlers[i].r)
		require.NoError(t, err)
	}
	leader.Configuration().PersistentPeers = leaderPeers

	return peers, handlers, nil
}
