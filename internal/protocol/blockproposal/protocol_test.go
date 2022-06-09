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

	testnetwork "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/network"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
)

const defaultTimeout = 1 * time.Second

var emptyProposal = &BlockProposal{
	SystemIdentifier:   nil,
	NodeIdentifier:     "",
	UnicityCertificate: nil,
	Transactions:       nil,
	Signature:          nil,
}

type request struct {
	req   *BlockProposal
	sleep bool
}

func (p *request) r(req *BlockProposal) {
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
		self    *network.Peer
		timeout time.Duration
		outCh   chan<- network.ReceivedMessage
	}

	outCh := make(chan<- network.ReceivedMessage, 1)
	defer close(outCh)
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "self is nil",
			args: args{
				self:    nil,
				timeout: defaultTimeout,
				outCh:   outCh,
			},
			wantErr: ErrPeerIsNil,
		},
		{
			name: "output ch is nil",
			args: args{
				self:    testnetwork.CreatePeer(t),
				timeout: defaultTimeout,
				outCh:   nil,
			},
			wantErr: ErrOutputChIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.self, tt.args.timeout, tt.args.outCh)
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

func TestSendRequest_RequestIsNil(t *testing.T) {
	outCh := make(chan<- network.ReceivedMessage, 1)
	defer close(outCh)
	bp, err := New(testnetwork.CreatePeer(t), defaultTimeout, outCh)
	require.NoError(t, err)
	require.ErrorIs(t, bp.Publish(nil), ErrRequestIsNil)
}

func TestSendRequest_SingleNodeOk(t *testing.T) {
	outCh := make(chan<- network.ReceivedMessage, 1)
	defer close(outCh)
	bp, err := New(testnetwork.CreatePeer(t), defaultTimeout, outCh)
	require.NoError(t, err)
	require.NoError(t, bp.Publish(emptyProposal))
}

func TestSendRequestToMultipleNodes_Ok(t *testing.T) {
	outCh := make(chan<- network.ReceivedMessage, 1)
	defer close(outCh)
	bp, err := New(testnetwork.CreatePeer(t), defaultTimeout, outCh)
	_, reqStores, err := createNodes(t, 4, bp.self)
	require.NoError(t, err)
	require.NoError(t, bp.Publish(emptyProposal))
	for _, r := range reqStores {
		require.Eventually(t, func() bool {
			req := <-r
			return proto.Equal(emptyProposal, req.Message)
		}, test.WaitDuration, test.WaitTick)
	}
}

func TestSendRequestToMultipleNodes_FollowerRefusesConnection(t *testing.T) {
	outCh := make(chan<- network.ReceivedMessage, 1)
	defer close(outCh)
	bp, err := New(testnetwork.CreatePeer(t), defaultTimeout, outCh)
	followers, requests, err := createNodes(t, 4, bp.self)

	require.NoError(t, err)
	err = followers[0].self.Close()
	require.NoError(t, err)
	err = bp.Publish(emptyProposal)
	require.True(t, strings.Contains(err.Error(), "failed to open stream"))
	for i := 1; i < 4; i++ {
		require.Eventually(t, func() bool {
			req := <-requests[i]
			return proto.Equal(emptyProposal, req.Message)
		}, test.WaitDuration, test.WaitTick)
	}

}

func createNodes(t *testing.T, nrOfNodes int, leader *network.Peer) ([]*Protocol, []chan network.ReceivedMessage, error) {
	peers := make([]*Protocol, nrOfNodes)
	outChs := make([]chan network.ReceivedMessage, nrOfNodes)
	leaderPeers := leader.Configuration().PersistentPeers
	for i := 0; i < nrOfNodes; i++ {
		peer := testnetwork.CreatePeer(t)
		pubKey, err := peer.PublicKey()
		require.NoError(t, err)

		pubKeyBytes, err := pubKey.Raw()
		require.NoError(t, err)

		leaderPeers = append(leaderPeers, &network.PeerInfo{
			Address:   fmt.Sprintf("%v", peer.MultiAddresses()),
			PublicKey: pubKeyBytes,
		})
		leader.Network().Peerstore().SetAddrs(peer.ID(), peer.MultiAddresses(), peerstore.PermanentAddrTTL)
		outChs[i] = make(chan network.ReceivedMessage, 1)
		peers[i], err = New(peer, defaultTimeout, outChs[i])
		require.NoError(t, err)
	}
	leader.Configuration().PersistentPeers = leaderPeers

	return peers, outChs, nil
}
