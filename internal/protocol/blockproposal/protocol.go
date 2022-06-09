package blockproposal

import (
	"context"
	"sync"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	"github.com/hashicorp/go-multierror"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"google.golang.org/protobuf/proto"
)

const ProtocolBlockProposal = "/ab/block-proposal/0.0.1"

var (
	ErrPeerIsNil     = errors.New("self is nil")
	ErrOutputChIsNil = errors.New("output channel is nil")
	ErrRequestIsNil  = errors.New("block proposal request is nil")
	ErrTimout        = errors.New("timeout")
)

// Protocol is a block proposal protocol (a.k.a. PC1-O protocol). It is used by leader to send and receive block
// proposals from the network. See Alphabill yellowpaper for more information.
type Protocol struct {
	self    *network.Peer
	timeout time.Duration
	outCh   chan<- network.ReceivedMessage
}

// New creates an instance of block proposal protocol. See Alphabill yellowpaper for more information.
func New(self *network.Peer, timeout time.Duration, outCh chan<- network.ReceivedMessage) (*Protocol, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if outCh == nil {
		return nil, ErrOutputChIsNil
	}
	bp := &Protocol{
		self:    self,
		timeout: timeout,
		outCh:   outCh,
	}
	self.RegisterProtocolHandler(ProtocolBlockProposal, bp.HandleStream)
	return bp, nil
}

func (p *Protocol) ID() string {
	return ProtocolBlockProposal
}

func (p *Protocol) Send(m proto.Message, receiver peer.ID) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	s, err := p.self.CreateStream(ctx, receiver, ProtocolBlockProposal)
	if err != nil {
		return err
	}
	defer func() {
		err = s.Close()
		if err != nil {
			logger.Warning("Failed to close stream: %v", err)
		}
	}()
	w := protocol.NewProtoBufWriter(s)
	if err := w.Write(m); err != nil {
		_ = s.Reset()
		return err
	}
	return nil
}

// Publish sends BlockProposal to all validator nodes in the network. NB! This method blocks until a response is received
// from all validators.
func (p *Protocol) Publish(req *BlockProposal) error {
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
		logger.Debug("Sending proposal to peer %v", id)
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

func (p *Protocol) send(req *BlockProposal, peerID peer.ID, responses chan error, wg *sync.WaitGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	responseCh := make(chan error, 1)

	go func() {
		defer close(responseCh)
		defer cancel()
		s, err := p.self.CreateStream(ctx, peerID, ProtocolBlockProposal)
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
		responseCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("Block proposal timeout: receiver: %v, sender %v", peerID, p.self.ID())
		responses <- ErrTimout
		wg.Done()
	case err := <-responseCh:
		if err != nil {
			responses <- err
		}
		wg.Done()
	}
}

func (p *Protocol) HandleStream(s libp2pNetwork.Stream) {
	r := protocol.NewProtoBufReader(s)
	defer func() {
		err := r.Close()
		if err != nil {
			logger.Warning("Failed to close protobuf stream: %v", err)
		}
	}()

	proposal := &BlockProposal{}
	err := r.Read(proposal)
	if err != nil {
		logger.Warning("Failed to read the block proposal request: %v", err)
		return
	}
	p.outCh <- network.ReceivedMessage{
		From:     s.Conn().RemotePeer(),
		Protocol: ProtocolBlockProposal,
		Message:  proposal,
	}
}

func (p *Protocol) Close() {
	p.self.RemoveProtocolHandler(ProtocolBlockProposal)
}
