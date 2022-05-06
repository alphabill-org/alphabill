package partition

import (
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/libp2p/go-libp2p-core/peerstore"

	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/testnetwork"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
	"github.com/libp2p/go-libp2p-core/peer"
)

const capacity = 10

func TestNewBlockCertificationSubscriber_NotOk(t *testing.T) {
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
			got, err := NewBlockCertificationSubscriber(tt.args.self, tt.args.rootNodeID, capacity, tt.args.eb)
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

type testCertificationRequestHandler struct {
	requestReceived bool
}

func (h *testCertificationRequestHandler) handle(s libp2pNetwork.Stream) {
	h.requestReceived = true

	w := protocol.NewProtoBufWriter(s)
	w.Write(&p1.P1Response{
		Status:  p1.P1Response_OK,
		Message: &certificates.UnicityCertificate{},
	})
	err := w.Close()
	if err != nil {
		logger.Warning("Failed to close stream")
	}
}

func TestSendBlockCertificationRequest_Ok(t *testing.T) {
	peer := testnetwork.CreatePeer(t)
	root := testnetwork.CreatePeer(t)
	handler := testCertificationRequestHandler{}
	root.RegisterProtocolHandler(p1.ProtocolP1, handler.handle)
	peer.Network().Peerstore().AddAddrs(root.ID(), root.MultiAddresses(), peerstore.PermanentAddrTTL)

	eb := eventbus.New()
	ch, err := eb.Subscribe(eventbus.TopicPartitionUnicityCertificate, 1)
	require.NoError(t, err)
	sub, err := NewBlockCertificationSubscriber(peer, root.ID(), capacity, eb)
	require.NoError(t, err)
	defer sub.Close()
	err = eb.Submit(eventbus.TopicP1, eventbus.BlockCertificationEvent{Req: &p1.P1Request{}})
	require.NoError(t, err)

	// wait request
	require.Eventually(t, func() bool {
		return handler.requestReceived
	}, test.WaitDuration, test.WaitTick)

	// wait response
	require.Eventually(t, func() bool {
		e := <-ch
		return e != nil
	}, test.WaitDuration, test.WaitTick)
}
