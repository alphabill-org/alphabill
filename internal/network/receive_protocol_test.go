package network

import (
	"context"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const testProtocolID = "/ab/test/0.0.1"

var testTypeFn = func() *txsystem.Transaction { return &txsystem.Transaction{} }

func TestNewReceiverProtocol_Ok(t *testing.T) {
	sender := createPeer(t)
	defer func() { require.NoError(t, sender.Close()) }()
	receiver := createPeer(t)
	defer func() { require.NoError(t, receiver.Close()) }()
	sender.Network().Peerstore().AddAddrs(receiver.ID(), receiver.MultiAddresses(), peerstore.PermanentAddrTTL)

	ch := make(chan ReceivedMessage, 1)
	defer close(ch)

	p, err := NewReceiverProtocol[*txsystem.Transaction](receiver, testProtocolID, ch, testTypeFn)
	require.Nil(t, err)
	defer p.Close()

	s, err := sender.CreateStream(context.Background(), receiver.ID(), testProtocolID)
	require.NoError(t, err)
	defer func() { require.NoError(t, s.Close()) }()

	w := NewProtoBufWriter(s)
	tx := testtransaction.RandomBillTransfer(t)
	err = w.Write(tx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		m := <-ch
		require.True(t, proto.Equal(tx, m.Message))
		return true
	}, test.WaitDuration, test.WaitTick)
}

func TestNewReceiverProtocol_NotOk(t *testing.T) {
	ch := make(chan ReceivedMessage, 1)
	defer close(ch)
	type args struct {
		self       *Peer
		protocolID string
		outCh      chan<- ReceivedMessage
		typeFunc   TypeFunc[*txsystem.Transaction]
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
				outCh:      ch,
				typeFunc:   testTypeFn,
			},
			wantErr: ErrStrPeerIsNil,
		},
		{
			name: "protocol ID is empty",
			args: args{
				self:       createPeer(t),
				protocolID: "",
				outCh:      ch,
				typeFunc:   testTypeFn,
			},
			wantErr: ErrStrProtocolIDEmpty,
		},
		{
			name: "out channel is nil",
			args: args{
				self:       createPeer(t),
				protocolID: testProtocolID,
				outCh:      nil,
				typeFunc:   testTypeFn,
			},
			wantErr: ErrStrOutputChIsNil,
		},
		{
			name: "type function is nil",
			args: args{
				self:       createPeer(t),
				protocolID: testProtocolID,
				outCh:      ch,
				typeFunc:   nil,
			},
			wantErr: ErrStrTypeFuncIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewReceiverProtocol[*txsystem.Transaction](tt.args.self, tt.args.protocolID, tt.args.outCh, tt.args.typeFunc)
			require.ErrorContains(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}
