package network

import (
	"strings"
	"testing"
	"time"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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

	err = peer2Protocol.Send(testtransaction.RandomBillTransfer(), peer1.ID())
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to open stream")
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

	err = peer2Protocol.Send(testtransaction.RandomBillTransfer(), peer1.ID())
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "connection refused"))
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
	p, err := NewSendProtocol(peer2, testProtocolID, 40*time.Millisecond)
	require.NoError(t, err)

	ch := make(chan ReceivedMessage)
	defer close(ch)

	// init receive protocol
	receive, err := NewReceiverProtocol[*txsystem.Transaction](peer1, testProtocolID, ch, func() *txsystem.Transaction {
		return &txsystem.Transaction{}
	})
	require.NoError(t, err)
	defer receive.Close()

	// peer2 forwards tx to peer1
	transfer := testtransaction.RandomBillTransfer()
	err = p.Send(transfer, peer1.ID())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		m := <-ch
		require.Equal(t, testProtocolID, m.Protocol)
		require.True(t, proto.Equal(transfer, m.Message))
		return true
	}, test.WaitDuration, test.WaitTick)
}
