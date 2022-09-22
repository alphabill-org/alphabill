package network

import (
	"context"
	"time"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

func NewSendProtocol(self *Peer, protocolID string, timeout time.Duration) (*SendProtocol, error) {
	if self == nil {
		return nil, errors.New(ErrStrPeerIsNil)
	}
	if protocolID == "" {
		return nil, errors.New(ErrStrProtocolIDEmpty)
	}

	p := &SendProtocol{protocol: &protocol{self: self, protocolID: protocolID}, timeout: timeout}
	return p, nil
}

func (p *SendProtocol) ID() string {
	return p.protocolID
}

func (p *SendProtocol) Send(m proto.Message, receiverID peer.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	doneCh := make(chan error, 1)
	go func() {
		defer close(doneCh)
		defer cancel()
		s, err := p.self.CreateStream(ctx, receiverID, p.protocolID)
		if err != nil {
			doneCh <- errors.Wrapf(err, "failed to open stream: "+
				"protocol: %s, receiver peerID: %v, sender peerID: %v", p.protocolID, receiverID, p.self.ID())
			return
		}
		defer func() {
			err := s.Close()
			if err != nil {
				logger.Warning("Failed to close libp2p stream. Error %v, "+
					"protocol: %s, receiver peerID: %v, sender peerID: %v", err, p.protocolID, receiverID, p.self.ID())
			}
		}()

		w := NewProtoBufWriter(s)
		defer func() {
			err := w.Close()
			if err != nil {
				logger.Warning("Failed to close protobuf writer. Error %v, "+
					"protocol: %s, receiver peerID: %v, sender peerID: %v", err, p.protocolID, receiverID, p.self.ID())
			}
		}()
		err = w.Write(m)
		if err != nil {
			doneCh <- errors.Errorf("failed to write request: %v, "+
				"protocol: %s, receiver peerID: %v, sender peerID: %v", err, p.protocolID, receiverID, p.self.ID())
		}
		doneCh <- nil
	}()

	select {
	case <-ctx.Done():
		return errors.Errorf("timeout: protocol: %v, receiver peerID: %v, sender peerID: %v\"",
			p.protocolID, receiverID, p.self.ID())
	case err := <-doneCh:
		if err != nil {
			logger.Warning("sending message failed: %v, protocol: %s, receiver peerID: %v, sender peerID: %v",
				err, p.protocolID, receiverID, p.self.ID())
			return errors.Wrapf(err, "message sending failed: protocol %s, receiver peer ID: %v", p.protocolID, receiverID)
		}
		return nil
	}
}
