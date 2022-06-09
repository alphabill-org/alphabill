package certificates

import (
	"context"
	"sync"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	"github.com/hashicorp/go-multierror"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

const ProtocolReceiveUnicityCertificate = "/ab/certificates/0.0.1"

// Protocol is an unicity certificate broadcast protocol. It is used by the root chain to broadcast unicity certificates
type Protocol struct {
	self     *network.Peer
	timeout  time.Duration
	outputCh chan<- network.ReceivedMessage
}

func (p *Protocol) ID() string {
	return ProtocolReceiveUnicityCertificate
}

func NewSenderProtocol(self *network.Peer, timeout time.Duration) (*Protocol, error) {

	if self == nil {
		return nil, errors.New(errstr.NilArgument)
	}
	return &Protocol{
		self:    self,
		timeout: timeout,
	}, nil
}

func NewReceiverProtocol(self *network.Peer, outputCh chan<- network.ReceivedMessage) (*Protocol, error) {
	if self == nil {
		return nil, errors.New(errstr.NilArgument)
	}
	if outputCh == nil {
		return nil, errors.New(errstr.NilArgument)
	}

	p := &Protocol{
		self:     self,
		outputCh: outputCh,
	}
	self.RegisterProtocolHandler(ProtocolReceiveUnicityCertificate, p.HandleStream)
	return p, nil
}

func (p *Protocol) Close() {
	p.self.RemoveProtocolHandler(ProtocolReceiveUnicityCertificate)
}

func (p *Protocol) Send(uc any, nodes []peer.ID) error {
	if uc == nil {
		return errors.New("p1 response is nil")
	}
	wg := &sync.WaitGroup{}
	responses := make(chan error, len(nodes))

	var err error
	for _, peerID := range nodes {
		logger.Debug("Sending UC to the peer %v", peerID)
		wg.Add(1)
		go p.send(uc, peerID, responses, wg)
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

func (p *Protocol) send(req any, peerID peer.ID, responses chan error, wg *sync.WaitGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	responseCh := make(chan error, 1)

	go func() {
		defer close(responseCh)
		defer cancel()
		s, err := p.self.CreateStream(ctx, peerID, ProtocolReceiveUnicityCertificate)
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
			responseCh <- errors.Errorf("failed to send uc %v", err)
			return
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("UC sending timeout: receiver: %v, sender %v", peerID, p.self.ID())
		responses <- errors.Errorf("timeout: failed to send uc to the peer %v", peerID)
		wg.Done()
	case err := <-responseCh:
		responses <- err
		wg.Done()
	}
}

func (p *Protocol) HandleStream(s libp2pNetwork.Stream) {

}
