package network

import (
	"context"
	"testing"
	"time"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/stretchr/testify/require"
)

func TestNewSendProtocol_NotOk(t *testing.T) {
	type args struct {
		self       *Peer
		protocolID string
		timeout    time.Duration
	}
	tests := []struct {
		name    string
		args    args
		wantErr string
	}{
		{
			name: "peer is nil",
			args: args{
				self:       nil,
				protocolID: testProtocolID,
				timeout:    time.Second,
			},
			wantErr: ErrStrPeerIsNil,
		},
		{
			name: "protocol ID is empty",
			args: args{
				self:       createPeer(t),
				protocolID: "",
				timeout:    time.Second,
			},
			wantErr: ErrStrProtocolIDEmpty,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSendProtocol(tt.args.self, tt.args.protocolID, tt.args.timeout)
			require.ErrorContains(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

func TestSend_UnknownPeer(t *testing.T) {
	peer1 := createPeer(t)
	defer peer1.Close()
	peer2 := createPeer(t)
	defer peer2.Close()

	peer2Protocol, err := NewSendProtocol(peer2, testProtocolID, time.Second)
	require.NoError(t, err)

	err = peer2Protocol.Send(moneytesttx.RandomBillTransfer(t), peer1.ID())
	require.Error(t, err)
	require.ErrorContains(t, err, "open stream error")
}

func TestSend_ConnectionRefused(t *testing.T) {
	// init peer1
	peer1 := createPeer(t)
	defer peer1.Close()

	// init peer1
	peer2 := createPeer(t)
	defer peer2.Close()

	// init peerstores
	peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
	peer2.Network().Peerstore().AddAddrs(peer1.ID(), peer1.MultiAddresses(), peerstore.PermanentAddrTTL)
	require.NoError(t, peer1.Close())

	// init send protocol
	peer2Protocol, err := NewSendProtocol(peer2, testProtocolID, time.Second)
	require.NoError(t, err)

	err = peer2Protocol.Send(moneytesttx.RandomBillTransfer(t), peer1.ID())
	require.Error(t, err)
	require.ErrorContains(t, err, "connection refused")
}

func TestSend_Ok(t *testing.T) {
	// init peer1
	peer1 := createPeer(t)
	defer peer1.Close()

	// init peer2
	peer2 := createPeer(t)
	defer peer2.Close()

	// init peerstores
	peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
	peer2.Network().Peerstore().AddAddrs(peer1.ID(), peer1.MultiAddresses(), peerstore.PermanentAddrTTL)

	// init send protocol
	p, err := NewSendProtocol(peer2, testProtocolID, 120*time.Millisecond)
	require.NoError(t, err)

	ch := make(chan ReceivedMessage)
	defer close(ch)

	// init receive protocol
	receive, err := NewReceiverProtocol[*txsystem.Transaction](peer1, testProtocolID, ch, func() *txsystem.Transaction {
		return &txsystem.Transaction{}
	})
	require.NoError(t, err)
	defer receive.Close()
	ctx := context.Background()
	con, err := peer2.Network().DialPeer(ctx, peer1.ID())
	require.NoError(t, err, "dial failed rom address %v to address %v", peer2.MultiAddresses(), peer1.MultiAddresses())
	defer con.Close()

	require.NotEmpty(t, peer2.Network().Conns())

	// peer2 forwards tx to peer1
	transfer := moneytesttx.RandomBillTransfer(t)
	require.NoError(t, p.Send(transfer, peer1.ID()))
	require.Eventually(t, func() bool {
		m := <-ch
		require.Equal(t, testProtocolID, m.Protocol)
		require.Equal(t, transfer, m.Message)
		return true
	}, test.WaitDuration, test.WaitTick)
}
