package p1

import (
	"context"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"github.com/libp2p/go-libp2p-core/peerstore"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	"github.com/stretchr/testify/require"

	testnetwork "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/network"
)

func TestSendP1Request(t *testing.T) {
	requestHandler := make(chan *RequestEvent)
	server := testnetwork.CreatePeer(t)
	defer server.Close()
	p1, err := NewRootChainCertificationProtocol(server, requestHandler)
	require.NoError(t, err)

	defer p1.Close()
	client := testnetwork.CreatePeer(t)
	defer client.Close()

	client.Network().Peerstore().AddAddr(server.ID(), server.MultiAddresses()[0], peerstore.PermanentAddrTTL)
	s, err := client.CreateStream(context.Background(), server.ID(), ProtocolP1)
	require.NoError(t, err)
	defer s.Close()

	w := protocol.NewProtoBufWriter(s)
	req := &P1Request{
		SystemIdentifier: nil,
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord:      nil,
	}
	err = w.Write(req)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		req := <-requestHandler
		req.ResponseCh <- &P1Response{}
		return true
	}, test.WaitDuration, test.WaitTick)

	r := protocol.NewProtoBufReader(s)
	defer r.Close()
	resp := &P1Request{}
	err = r.Read(resp)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestNew_InvalidInputs(t *testing.T) {
	type args struct {
		self           *network.Peer
		requestHandler chan<- *RequestEvent
	}
	tests := []struct {
		name string
		args args
		err  error
	}{
		{
			name: "peer is nil",
			args: args{
				self:           nil,
				requestHandler: make(chan *RequestEvent),
			},
			err: ErrPeerIsNil,
		},
		{
			name: "request handler is nil",
			args: args{
				self:           testnetwork.CreatePeer(t),
				requestHandler: nil,
			},
			err: ErrRequestHandlerIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRootChainCertificationProtocol(tt.args.self, tt.args.requestHandler)
			require.ErrorIs(t, err, tt.err)
			require.Nil(t, got)
		})
	}
}
