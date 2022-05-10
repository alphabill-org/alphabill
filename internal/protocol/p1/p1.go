package p1

import (
	"context"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"

	"github.com/libp2p/go-libp2p-core/peer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	log "gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
)

const ProtocolP1 = "/ab/root/p1/1.0.0"

var ErrPeerIsNil = errors.New("self is nil")
var ErrRequestHandlerIsNil = errors.New("request handler is nil")
var ErrResponseHandlerIsNil = errors.New("response handler is nil")
var logger = log.CreateForPackage()

// P1 is a block certification protocol. It is used by partition nodes to certify a new block.
// See Alphabill yellowpaper for more information
type P1 struct {
	self            *network.Peer
	ctx             context.Context
	responseHandler func(response *P1Response)
	ctxCancel       context.CancelFunc
	requestHandler  chan<- *RequestEvent
}

type RequestEvent struct {
	Req        *P1Request
	ResponseCh chan *P1Response
}

func NewRootChainCertificationProtocol(self *network.Peer, requestHandler chan<- *RequestEvent) (*P1, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if requestHandler == nil {
		return nil, ErrRequestHandlerIsNil
	}
	p1 := &P1{
		self:           self,
		requestHandler: requestHandler,
	}
	p1.ctx, p1.ctxCancel = context.WithCancel(context.Background())
	self.RegisterProtocolHandler(ProtocolP1, p1.handleStream)
	return p1, nil
}

func NewPartitionNodeCertificationProtocol(self *network.Peer, responseHandler func(response *P1Response)) (*P1, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if responseHandler == nil {
		return nil, ErrResponseHandlerIsNil
	}
	p1 := &P1{
		self:            self,
		responseHandler: responseHandler,
	}
	p1.ctx, p1.ctxCancel = context.WithCancel(context.Background())
	return p1, nil
}

func (p *P1) Submit(req *P1Request, rootNodeID peer.ID) error {
	if p.responseHandler == nil {
		return ErrResponseHandlerIsNil
	}
	if req == nil {
		return errors.New(errstr.NilArgument)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	errorCh := make(chan error, 1)
	responseCh := make(chan *P1Response, 1)
	go func() {
		defer close(responseCh)
		defer close(errorCh)
		defer cancel()
		s, err := p.self.CreateStream(ctx, rootNodeID, ProtocolP1)
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
		err = w.Write(req)
		if err != nil {
			errorCh <- errors.Errorf("failed to write request: %v", err)
		}

		reader := protocol.NewProtoBufReader(s)
		defer func() {
			err = reader.Close()
			if err != nil {
				errorCh <- errors.Errorf("failed to close protobuf reader: %v", err)
			}
		}()
		response := &P1Response{}
		err = reader.Read(response)
		if err != nil {
			errorCh <- errors.Errorf("failed to read response: %v", err)
		}
		responseCh <- response
	}()

	select {
	case <-ctx.Done():
		logger.Info("block certification timeout")
		return errors.New("timeout")
	case err := <-errorCh:
		logger.Info("requesting block certification failed: %v", err)
		return err
	case response := <-responseCh:
		p.responseHandler(response)
	}

	return nil
}

func (p *P1) handleStream(s libp2pNetwork.Stream) {
	r := protocol.NewProtoBufReader(s)
	defer func() {
		err := r.Close()
		if err != nil {
			logger.Warning("Failed to close protobuf reader: %v", err)
		}
	}()
	req := &P1Request{}
	err := r.Read(req)
	if err != nil {
		logger.Warning("Failed to read request: %v", err)
		return
	}
	responseChannel := make(chan *P1Response, 1)
	defer close(responseChannel)

	// send request to requestHandler
	select {
	case p.requestHandler <- &RequestEvent{Req: req, ResponseCh: responseChannel}:
	case <-p.ctx.Done():
		return
	}

	var response *P1Response
	// wait response or cancel
	select {
	case response = <-responseChannel:
	case <-p.ctx.Done():
		return
	}

	if response == nil {
		err := s.Reset()
		if err != nil {
			logger.Warning("Failed to reset libp2p stream: %v", err)
		}
		return
	}
	// write response message
	writer := protocol.NewProtoBufWriter(s)
	defer func() {
		err := writer.Close()
		if err != nil {
			logger.Warning("Failed to close protobuf writer: %v", err)
		}
	}()

	err = writer.Write(response)
	if err != nil {
		logger.Warning("Failed to write response: %v", err)
	}
}

func (p *P1) Close() {

	p.self.RemoveProtocolHandler(ProtocolP1)
	p.ctxCancel()
}
