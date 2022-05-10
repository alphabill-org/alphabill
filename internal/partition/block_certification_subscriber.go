package partition

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
)

type CertificationRequestSubscriber struct {
	self       *network.Peer
	eventbus   *eventbus.EventBus
	protocol   *p1.P1
	requestCh  <-chan interface{}
	ctx        context.Context
	ctxCancel  context.CancelFunc
	rootNodeID peer.ID
}

func NewBlockCertificationSubscriber(self *network.Peer, rootNodeID peer.ID, capacity uint, eb *eventbus.EventBus) (*CertificationRequestSubscriber, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if eb == nil {
		return nil, ErrEventBusIsNil
	}
	requestCh, err := eb.Subscribe(eventbus.TopicP1, capacity)
	if err != nil {
		return nil, err
	}
	c := &CertificationRequestSubscriber{
		self:       self,
		rootNodeID: rootNodeID,
		eventbus:   eb,
		requestCh:  requestCh,
	}
	c.protocol, err = p1.NewPartitionNodeCertificationProtocol(self, c.responseHandler)
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())

	go c.loop()
	return c, nil
}

func (c *CertificationRequestSubscriber) loop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case e := <-c.requestCh:
			converted, req := convertType[eventbus.BlockCertificationEvent](e)
			if !converted {
				logger.Warning("Invalid certification event: %v", e)
				continue
			}
			logger.Info("Request received: %v", req)
			err := c.protocol.Submit(req.Req, c.rootNodeID)
			if err != nil {
				logger.Warning("Failed to send certification request %v to the root node %v: %v", e, c.rootNodeID, err)
			}
		}
	}
}

func (c *CertificationRequestSubscriber) responseHandler(response *p1.P1Response) {
	if response.Status != p1.P1Response_OK {
		logger.Warning("Unexpected certification request response status: %s", response.Status)
		return
	}
	err := c.eventbus.Submit(eventbus.TopicPartitionUnicityCertificate, eventbus.UnicityCertificateEvent{
		Certificate: response.Message,
	})
	if err != nil {
		logger.Warning("Failed to submit unicity certificate: %v", err)
	}
}

func (c *CertificationRequestSubscriber) Close() {
	c.ctxCancel()
	c.protocol.Close()
}
