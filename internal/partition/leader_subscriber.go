package partition

import (
	"context"
	"sync"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/forwarder"

	"github.com/libp2p/go-libp2p-core/peer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txbuffer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
)

const defaultCapacity = 10

var (
	ErrUnknownPeerID    = errors.New("unknown peer ID")
	ErrTxBufferIsNil    = errors.New("tx buffer is nil")
	ErrTxForwarderIsNil = errors.New("tx forwarder is nil")
)

// LeaderSubscriber handles leader change events.
type LeaderSubscriber struct {
	self          peer.ID
	eb            *eventbus.EventBus
	buffer        *txbuffer.TxBuffer
	forwarder     *forwarder.TxForwarder
	leadersCh     <-chan interface{}
	ctx           context.Context
	cancel        context.CancelFunc
	txCtx         context.Context
	txCancel      context.CancelFunc
	currentLeader peer.ID
	wg            *sync.WaitGroup
}

func NewLeaderSubscriber(self peer.ID, eb *eventbus.EventBus, buffer *txbuffer.TxBuffer, forwarder *forwarder.TxForwarder) (*LeaderSubscriber, error) {
	if self == UnknownLeader {
		return nil, ErrUnknownPeerID
	}
	if eb == nil {
		return nil, ErrEventBusIsNil
	}
	if buffer == nil {
		return nil, ErrTxBufferIsNil
	}
	if forwarder == nil {
		return nil, ErrTxForwarderIsNil
	}

	leadersCh, err := eb.Subscribe(eventbus.TopicLeaders, defaultCapacity)
	if err != nil {
		return nil, err
	}

	l := &LeaderSubscriber{
		self:          self,
		wg:            &sync.WaitGroup{},
		eb:            eb,
		buffer:        buffer,
		forwarder:     forwarder,
		leadersCh:     leadersCh,
		currentLeader: UnknownLeader,
	}
	l.ctx, l.cancel = context.WithCancel(context.Background())
	go l.loop()
	return l, nil
}

func (lh *LeaderSubscriber) Close() {
	if lh.txCtx != nil {
		lh.txCancel()
	}
	lh.cancel()
}

func (lh *LeaderSubscriber) loop() {
	for {
		select {
		case <-lh.ctx.Done():
			logger.Info("Exiting LeaderSubscriber main loop")
			return
		case e := <-lh.leadersCh:
			logger.Info("Leader change event received: %v", e)
			lh.handleNewLeaderEvent(e)
		}
	}
}

func (lh *LeaderSubscriber) handleNewLeaderEvent(event interface{}) {
	switch event.(type) {
	case eventbus.NewLeaderEvent:
		lh.currentLeader = event.(eventbus.NewLeaderEvent).NewLeader
		if lh.txCtx != nil {
			// stop sending transactions
			lh.txCancel()
			lh.wg.Wait()
			lh.txCtx = nil
			lh.txCancel = nil
		}
		if lh.currentLeader == UnknownLeader {
			return
		}
		lh.txCtx, lh.txCancel = context.WithCancel(context.Background())
		lh.wg.Add(1)
		go lh.buffer.Process(lh.txCtx, lh.wg, lh.processTx)
	default:
		logger.Warning("Invalid NewLeaderEvent event: %v", event)
	}
}

func (lh *LeaderSubscriber) processTx(tx *transaction.Transaction) bool {
	if lh.self == lh.currentLeader {
		if err := lh.eb.Submit(eventbus.TopicPartitionTransaction, eventbus.TransactionEvent{Transaction: tx}); err != nil {
			return false
		}
	} else {
		if err := lh.forwarder.Forward(tx, lh.currentLeader); err != nil {
			return false
		}
	}
	return true
}
