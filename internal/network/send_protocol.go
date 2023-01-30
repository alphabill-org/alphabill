package network

import (
	"context"
	"fmt"
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
			doneCh <- fmt.Errorf("open stream error: %w", err)
			return
		}
		defer func() {
			err := s.Close()
			if err != nil {
				logger.Warning("Failed to close libp2p stream, error: %v, "+
					"protocol: %s, receiver peerID: %v, sender peerID: %v", err, p.protocolID, receiverID, p.self.ID())
			}
		}()

		w := NewProtoBufWriter(s)
		defer func() {
			err := w.Close()
			if err != nil {
				logger.Warning("Failed to close protobuf writer, error: %v, "+
					"protocol: %s, receiver peerID: %v, sender peerID: %v", err, p.protocolID, receiverID, p.self.ID())
			}
		}()
		err = w.Write(m)
		if err != nil {
			doneCh <- fmt.Errorf("write error: %w", err)
		}
		doneCh <- nil
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("send timeout")
	case err := <-doneCh:
		return err
	}
}
