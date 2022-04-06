package p1

import (
	"context"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	log "gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
)

const ProtocolP1 = "/ab/root/p1/1.0.0"

var ErrPeerIsNil = errors.New("self is nil")
var ErrRequestHandlerIsNil = errors.New("request handler is nil")
var logger = log.CreateForPackage()

// P1 is a block certification protocol. It is used by partition nodes to certify a new block.
// See Alphabill yellowpaper for more information
type P1 struct {
	self           *network.Peer
	ctx            context.Context
	ctxCancel      context.CancelFunc
	requestHandler chan<- *RequestEvent
}

type RequestEvent struct {
	Req        *P1Request
	ResponseCh chan *P1Response
}

func New(self *network.Peer, requestHandler chan<- *RequestEvent) (*P1, error) {
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
