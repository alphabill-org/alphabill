package certification

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/libp2p/go-libp2p-core/peer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	log "gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
)

const ProtocolBlockCertification = "/ab/block-certification/0.0.1"

var (
	ErrPeerIsNil               = errors.New("self is nil")
	ErrRequestHandlerIsNil     = errors.New("request handler is nil")
	ErrResponseHandlerIsNil    = errors.New("response handler is nil")
	ErrClientPeerIsNil         = errors.New("peer client is nil")
	ErrSignerIsNil             = errors.New("client signer is nil")
	ErrInvalidSystemIdentifier = errors.New("invalid system identifier")
)
var logger = log.CreateForPackage()

// Protocol is a block certification protocol. It is used by partition nodes to certify a new block.
// See Alphabill yellowpaper for more information
type Protocol struct {
	self           *network.Peer
	ctx            context.Context
	ctxCancel      context.CancelFunc
	requestHandler chan<- any
}

func NewReceiverProtocol(self *network.Peer, requestHandler chan<- any) (*Protocol, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if requestHandler == nil {
		return nil, ErrRequestHandlerIsNil
	}
	p1 := &Protocol{
		self:           self,
		requestHandler: requestHandler,
	}
	p1.ctx, p1.ctxCancel = context.WithCancel(context.Background())
	self.RegisterProtocolHandler(ProtocolBlockCertification, p1.HandleStream)
	return p1, nil
}

func NewSenderProtocol(self *network.Peer, timeout time.Duration) (*Protocol, error) {
	// TODO timeout
	if self == nil {
		return nil, ErrPeerIsNil
	}
	p := &Protocol{self: self}
	p.ctx, p.ctxCancel = context.WithCancel(context.Background())
	return p, nil
}

func (p *Protocol) ID() string {
	return ProtocolBlockCertification
}

func (p *Protocol) Send(m proto.Message, rootNodeID peer.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	errorCh := make(chan error, 1)
	go func() {
		defer close(errorCh)
		defer cancel()
		s, err := p.self.CreateStream(ctx, rootNodeID, ProtocolBlockCertification)
		if err != nil {
			errorCh <- errors.Wrap(err, "failed to open stream")
			return
		}
		defer func() {
			err := s.Close()
			if err != nil {
				logger.Warning("Failed to close libp2p stream: %v", err)
			}
		}()

		w := protocol.NewProtoBufWriter(s)
		defer func() {
			err := w.Close()
			if err != nil {
				logger.Warning("Failed to close protobuf writer: %v", err)
			}
		}()
		err = w.Write(m)
		if err != nil {
			errorCh <- errors.Errorf("failed to write request: %v", err)
		}
		errorCh <- nil

	}()

	select {
	case <-ctx.Done():
		logger.Info("block certification timeout")
		return errors.New("timeout")
	case err := <-errorCh:
		if err != nil {
			logger.Info("requesting block certification failed: %v", err)
		}
		return err
	}

	return nil
}

func (p *Protocol) HandleStream(s libp2pNetwork.Stream) {

}

func (p *Protocol) Close() {
	p.self.RemoveProtocolHandler(ProtocolBlockCertification)
	p.ctxCancel()
}
