package blockproposal

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

const ProtocolBlockProposal = "/ab/root/pc1-O/1.0.0"

var (
	ErrPeerIsNil           = errors.New("self is nil")
	ErrRequestHandlerIsNil = errors.New("block proposal request handler is nil")
	ErrRequestIsNil        = errors.New("block proposal request is nil")
	ErrTimout              = errors.New("timeout")
)

// Protocol is a block proposal protocol (a.k.a. PC1-O protocol). It is used by leader to send and receive block
// proposals from the network. See Alphabill yellowpaper for more information.
type (
	Protocol struct {
		self           *network.Peer
		timeout        time.Duration
		requestHandler ProtocolHandler
	}

	ProtocolHandler func(req *BlockProposal)
)

// New creates an instance of block proposal protocol. See Alphabill yellowpaper for more information.
func New(self *network.Peer, timeout time.Duration, requestHandler ProtocolHandler) (*Protocol, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if requestHandler == nil {
		return nil, ErrRequestHandlerIsNil
	}
	bp := &Protocol{
		self:           self,
		timeout:        timeout,
		requestHandler: requestHandler,
	}
	self.RegisterProtocolHandler(ProtocolBlockProposal, bp.handleStream)
	return bp, nil
}

// Publish sends BlockProposal to all validator nodes in the network. NB! This method blocks until a response is received
// from all validators.
func (bp *Protocol) Publish(req *BlockProposal) error {
	if req == nil {
		return ErrRequestIsNil
	}
	persistentPeers := bp.self.Configuration().PersistentPeers
	wg := &sync.WaitGroup{}
	responses := make(chan error, len(persistentPeers))

	var err error
	for _, p := range persistentPeers {
		id, e := p.GetID()
		if e != nil {
			err = multierror.Append(err, e)
			continue
		}
		if id == bp.self.ID() {
			continue
		}
		wg.Add(1)
		go bp.send(req, id, responses, wg)
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

func (bp *Protocol) send(req *BlockProposal, peerID peer.ID, responses chan error, wg *sync.WaitGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), bp.timeout)
	responseCh := make(chan error, 1)

	go func() {
		defer close(responseCh)
		defer cancel()
		s, err := bp.self.CreateStream(ctx, peerID, ProtocolBlockProposal)
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
		logger.Info("PC1-O timeout: receiver: %v, sender %v", peerID, bp.self.ID())
		responses <- ErrTimout
		wg.Done()
	case err := <-responseCh:
		responses <- err
		wg.Done()
	}
}

func (bp *Protocol) handleStream(s libp2pNetwork.Stream) {
	r := protocol.NewProtoBufReader(s)
	defer func() {
		err := r.Close()
		if err != nil {
			logger.Warning("Failed to close protobuf stream: %v", err)
		}
	}()

	req := &BlockProposal{}
	err := r.Read(req)
	if err != nil {
		logger.Warning("Failed to read the PC10Request: %v", err)
		return
	}
	bp.requestHandler(req)
}
