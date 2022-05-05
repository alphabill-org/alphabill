package partition

import (
	"context"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
)

type BlockProposalSubscriber struct {
	self      *network.Peer
	eventbus  *eventbus.EventBus
	protocol  *blockproposal.Protocol
	outCh     <-chan interface{}
	ctx       context.Context
	ctxCancel context.CancelFunc
}

func NewBlockProposalSubscriber(self *network.Peer, capacity uint, timeout time.Duration, eb *eventbus.EventBus) (*BlockProposalSubscriber, error) {
	outCh, err := eb.Subscribe(eventbus.TopicBlockProposalOutput, capacity)
	if err != nil {
		return nil, err
	}
	c := &BlockProposalSubscriber{
		self:     self,
		eventbus: eb,
		outCh:    outCh,
	}
	c.protocol, err = blockproposal.New(self, timeout, c.handler)
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())

	go c.loop()
	return c, nil
}

func (c *BlockProposalSubscriber) loop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case e := <-c.outCh:
			converted, req := convertType[eventbus.BlockProposalEvent](e)
			if !converted {
				logger.Warning("Invalid certification event: %v", e)
				continue
			}
			logger.Info("Request received: %v", req)
			err := c.protocol.Publish(req.BlockProposal)
			if err != nil {
				logger.Warning("Failed to publish block proposal: %v", err)
			}
		}

	}
}

func (c *BlockProposalSubscriber) handler(proposal *blockproposal.BlockProposal) {
	// TODO what happens if status isn't OK????
	c.eventbus.Submit(eventbus.TopicBlockProposalInput, eventbus.BlockProposalEvent{
		BlockProposal: proposal,
	})
}

func (c *BlockProposalSubscriber) Close() {
	c.ctxCancel()
	// TODO
	// c.protocol.Close()
}
