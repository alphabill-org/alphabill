package protocol

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	testnetwork "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/network"
	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/stretchr/testify/require"
)

func TestNewSendProtocol_NotOk(t *testing.T) {
	type args struct {
		self       *network.Peer
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
				self:       testnetwork.CreatePeer(t),
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
	peer1 := testnetwork.CreatePeer(t)
	defer peer1.Close()
	peer2 := testnetwork.CreatePeer(t)
	defer peer2.Close()

	peer2Protocol, err := NewSendProtocol(peer2, testProtocolID, time.Second)
	require.NoError(t, err)

	err = peer2Protocol.Send(testtransaction.RandomBillTransfer(), peer1.ID())
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to open stream")
}

func TestSend_ConnectionRefused(t *testing.T) {
	// init peer1
	peer1 := testnetwork.CreatePeer(t)
	defer peer1.Close()

	// init peer1
	peer2 := testnetwork.CreatePeer(t)
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

func TestSend_Timeout(t *testing.T) {
	// init peer1
	peer1 := testnetwork.CreatePeer(t)
	defer peer1.Close()

	// init peer2
	peer2 := testnetwork.CreatePeer(t)
	defer peer2.Close()

	// init peerstores
	peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
	peer2.Network().Peerstore().AddAddrs(peer1.ID(), peer1.MultiAddresses(), peerstore.PermanentAddrTTL)

	// init send protocol
	p, err := NewSendProtocol(peer2, testProtocolID, 40*time.Millisecond)
	require.NoError(t, err)

	ch := make(chan network.ReceivedMessage, 1)
	defer close(ch)

	// init receive protocol
	receive, err := NewReceiverProtocol[*txsystem.Transaction](peer1, testProtocolID, ch, func() *txsystem.Transaction {
		time.Sleep(100 * time.Millisecond)
		return &txsystem.Transaction{}
	})
	require.NoError(t, err)
	defer receive.Close()

	// peer2 forwards tx to peer1
	err = p.Send(testtransaction.RandomBillTransfer(), peer1.ID())
	require.ErrorContains(t, err, fmt.Sprintf("timeout: protocol: %s", testProtocolID))
}
