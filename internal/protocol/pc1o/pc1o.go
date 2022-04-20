package pc1o

import (
	"context"
	"sync"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/hashicorp/go-multierror"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
)

const ProtocolPC1O = "/ab/root/pc1-O/1.0.0"

var (
	ErrPeerIsNil           = errors.New("self is nil")
	ErrRequestHandlerIsNil = errors.New("pc1-O request handler is nil")
	ErrRequestIsNil        = errors.New("pc1-O request is nil")
	ErrTimout              = errors.New("timeout")
)

// PC1O is a block proposal protocol. It is used by leader to send block proposals to followers.
// See Alphabill yellowpaper for more information.
type (
	PC1O struct {
		self           *network.Peer
		timeout        time.Duration
		requestHandler PC10RequestHandler
	}

	PC10RequestHandler func(req *PC1ORequest)
)

// New creates an instance of PC1-O protocol. See Alphabill yellowpaper for more information.
func New(self *network.Peer, timeout time.Duration, requestHandler PC10RequestHandler) (*PC1O, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if requestHandler == nil {
		return nil, ErrRequestHandlerIsNil
	}
	pc1o := &PC1O{
		self:           self,
		timeout:        timeout,
		requestHandler: requestHandler,
	}
	self.RegisterProtocolHandler(ProtocolPC1O, pc1o.handleStream)
	return pc1o, nil
}

// Publish sends PC1ORequest to all validator nodes in the network. NB! This method blocks until a response is received
// from all validators.
func (p *PC1O) Publish(req *PC1ORequest) error {
	if req == nil {
		return ErrRequestIsNil
	}
	persistentPeers := p.self.Configuration().PersistentPeers
	wg := &sync.WaitGroup{}
	responses := make(chan error, len(persistentPeers))

	var err error
	for _, peer := range persistentPeers {
		id, e := peer.GetID()
		if e != nil {
			err = multierror.Append(err, e)
			continue
		}
		if id == p.self.ID() {
			continue
		}
		wg.Add(1)
		go p.send(req, id, responses, wg)
	}
	// wait all responses
	wg.Wait()
	// close channel
	close(responses)

	// read responses from the channel. will terminate after receiving the responses.
	for e := range responses {
		if e != nil {
			err = multierror.Append(err, e)
		}
	}
	return err
}

func (p *PC1O) send(req *PC1ORequest, peerID peer.ID, responses chan error, wg *sync.WaitGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	responseCh := make(chan error, 1)

	go func() {
		defer close(responseCh)
		defer cancel()
		s, err := p.self.CreateStream(ctx, peerID, ProtocolPC1O)
		if err != nil {
			responseCh <- errors.Wrapf(err, "failed to open stream for peer %v", peerID)
			return
		}
		defer func() {
			responseCh <- s.Close()
		}()
		w := protocol.NewProtoBufWriter(s)
		if err := w.Write(req); err != nil {
			_ = s.Reset()
			responseCh <- errors.Errorf("failed to send block proposal %v", err)
			return
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("PC1-O timeout: receiver: %v, sender %v", peerID, p.self.ID())
		responses <- ErrTimout
		wg.Done()
	case err := <-responseCh:
		responses <- err
		wg.Done()
	}
}

func (p *PC1O) handleStream(s libp2pNetwork.Stream) {
	r := protocol.NewProtoBufReader(s)
	defer func() {
		err := r.Close()
		if err != nil {
			logger.Warning("Failed to close protobuf stream: %v", err)
		}
	}()

	req := &PC1ORequest{}
	err := r.Read(req)
	if err != nil {
		logger.Warning("Failed to read the PC10Request: %v", err)
		return
	}
	p.requestHandler(req)
}
