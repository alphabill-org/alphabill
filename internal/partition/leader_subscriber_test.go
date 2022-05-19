package partition

import (
	gocrypto "crypto"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/forwarder"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testnetwork "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/network"
	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txbuffer"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	transaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
)

const testPeerID = peer.ID("test")

func TestNewLeaderHandler_NotOk(t *testing.T) {
	txBuffer, err := txbuffer.New(10, gocrypto.SHA256)
	require.NoError(t, err)
	f, err := forwarder.New(testnetwork.CreatePeer(t), 10*time.Millisecond, func(tx *txsystem.Transaction) {
		require.Fail(t, "")
	})
	require.NoError(t, err)
	type args struct {
		self      peer.ID
		eb        *eventbus.EventBus
		buffer    *txbuffer.TxBuffer
		forwarder *forwarder.TxForwarder
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "leader is unknown",
			args: args{
				self: UnknownLeader,
			},
			wantErr: ErrUnknownPeerID,
		},
		{
			name: "eventbus is nil",
			args: args{
				self:      testPeerID,
				eb:        nil,
				buffer:    txBuffer,
				forwarder: f,
			},
			wantErr: ErrEventBusIsNil,
		},
		{
			name: "tx buffer is nil",
			args: args{
				self:      testPeerID,
				eb:        eventbus.New(),
				buffer:    nil,
				forwarder: f,
			},
			wantErr: ErrTxBufferIsNil,
		},
		{
			name: "tx forwarder is nil",
			args: args{
				self:      testPeerID,
				eb:        eventbus.New(),
				buffer:    txBuffer,
				forwarder: nil,
			},
			wantErr: ErrTxForwarderIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewLeaderSubscriber(tt.args.self, tt.args.eb, tt.args.buffer, tt.args.forwarder)
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

func TestNewLeaderHandler_SendsTxEvents(t *testing.T) {
	txBuffer1, eb1, lh1 := createLeaderHandler(t)
	transactionsCh, err := eb1.Subscribe(eventbus.TopicPartitionTransaction, 10)
	require.NoError(t, err)
	require.NoError(t, eb1.Submit(eventbus.TopicLeaders, eventbus.NewLeaderEvent{NewLeader: UnknownLeader}))
	require.NoError(t, txBuffer1.Add(testtransaction.RandomGenericBillTransfer(t)))
	require.NoError(t, txBuffer1.Add(testtransaction.RandomGenericBillTransfer(t)))
	require.NoError(t, txBuffer1.Add(testtransaction.RandomGenericBillTransfer(t)))
	require.Eventually(t, func() bool { return txBuffer1.Count() == uint32(3) }, test.WaitDuration, test.WaitTick)

	require.NoError(t, eb1.Submit(eventbus.TopicLeaders, eventbus.NewLeaderEvent{NewLeader: lh1.self}))
	require.Eventually(t, func() bool {
		<-transactionsCh
		<-transactionsCh
		<-transactionsCh
		return true
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, uint32(0), txBuffer1.Count())
	require.NoError(t, eb1.Submit(eventbus.TopicLeaders, eventbus.NewLeaderEvent{NewLeader: UnknownLeader}))
	require.Eventually(t, func() bool { return lh1.txCtx == nil }, test.WaitDuration, test.WaitTick)
}

func TestNewLeaderHandler_TxForwardingFails(t *testing.T) {
	txBuffer1, eb1, lh1 := createLeaderHandler(t)
	require.NoError(t, eb1.Submit(eventbus.TopicLeaders, eventbus.NewLeaderEvent{NewLeader: UnknownLeader}))
	require.Eventually(t, func() bool { return UnknownLeader == lh1.currentLeader }, test.WaitDuration, test.WaitTick)
	require.NoError(t, txBuffer1.Add(testtransaction.RandomGenericBillTransfer(t)))
	require.Eventually(t, func() bool { return txBuffer1.Count() == uint32(1) }, test.WaitDuration, test.WaitTick)
	require.NoError(t, eb1.Submit(eventbus.TopicLeaders, eventbus.NewLeaderEvent{NewLeader: testPeerID}))
	require.Eventually(t, func() bool { return txBuffer1.Count() == uint32(1) }, test.WaitDuration, test.WaitTick)
}

func createLeaderHandler(t *testing.T) (*txbuffer.TxBuffer, *eventbus.EventBus, *LeaderSubscriber) {
	txBuffer, err := txbuffer.New(10, gocrypto.SHA256)
	require.NoError(t, err)
	p := testnetwork.CreatePeer(t)
	f, err := forwarder.New(p, 10*time.Millisecond, func(tx *transaction.Transaction) {
		require.NoError(t, err)
	})
	require.NoError(t, err)
	eb := eventbus.New()
	lh, err := NewLeaderSubscriber(p.ID(), eb, txBuffer, f)
	go lh.Run()
	t.Cleanup(lh.Close)
	return txBuffer, eb, lh
}
