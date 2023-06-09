package network

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
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

func (p *SendProtocol) Send(m any, receiverID peer.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	doneCh := make(chan error, 1)

	go func() {
		s, err := p.self.CreateStream(ctx, receiverID, p.protocolID)
		if err != nil {
			doneCh <- fmt.Errorf("open stream error: %w", err)
			return
		}
		defer func() {
			if err := s.Close(); err != nil {
				logger.Warning("Failed to close libp2p stream, error: %v, "+
					"protocol: %s, receiver peerID: %v, sender peerID: %v", err, p.protocolID, receiverID, p.self.ID())
			}
		}()
		w := NewCBORWriter(s)
		defer func() {
			if err := w.Close(); err != nil {
				logger.Warning("Failed to close protobuf writer, error: %v, "+
					"protocol: %s, receiver peerID: %v, sender peerID: %v", err, p.protocolID, receiverID, p.self.ID())
			}
		}()

		if err := w.Write(m); err != nil {
			doneCh <- fmt.Errorf("failed to write request: %v, "+
				"protocol: %s, receiver peerID: %v, sender peerID: %v", err, p.protocolID, receiverID, p.self.ID())
		}
		doneCh <- nil
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout: protocol: %v, receiver peerID: %v, sender peerID: %v",
			p.protocolID, receiverID, p.self.ID())
	case err := <-doneCh:
		return err
	}
}
