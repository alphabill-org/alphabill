package forwarder

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

var (
	ErrPeerIsNil              = errors.New("peer is nil")
	ErrReceivedMessageChIsNil = errors.New("received message channel is nil")
	ErrTimout                 = errors.New("forwarding timeout")
)

const (
	// ProtocolInputForward is the protocol.ID of the Alphabill input forwarding protocol.
	ProtocolInputForward = "/ab/input-forward/0.0.1"
)

type (

	// TxForwarder sends transactions to the expected leader.
	TxForwarder struct {
		self               *network.Peer
		receivedMessagesCh chan<- network.ReceivedMessage
		timeout            time.Duration
	}

	TxHandler func(tx *txsystem.Transaction)
)

func New(self *network.Peer, timeout time.Duration, receivedMessagesCh chan<- network.ReceivedMessage) (*TxForwarder, error) {
	//protocol.NewSendProtocol(self, ProtocolInputForward, timeout)

	if self == nil {
		return nil, ErrPeerIsNil
	}
	if receivedMessagesCh == nil {
		return nil, ErrReceivedMessageChIsNil
	}
	t := &TxForwarder{self: self, receivedMessagesCh: receivedMessagesCh, timeout: timeout}
	self.RegisterProtocolHandler(ProtocolInputForward, t.HandleStream)
	return t, nil
}

func (tf *TxForwarder) ID() string {
	return ProtocolInputForward
}

func (tf *TxForwarder) Send(message proto.Message, receiverID peer.ID) error {
	m, f := message.(*txsystem.Transaction)
	if !f {
		return errors.Errorf("invalid message type. expected: %T, got %T", &txsystem.Transaction{}, message)
	}
	return tf.Forward(m, receiverID)
}

// Forward sends the transaction to the receiver.
func (tf *TxForwarder) Forward(req *txsystem.Transaction, receiver peer.ID) error {
	if req == nil {
		return errors.New(errstr.NilArgument)
	}
	ctx, cancel := context.WithTimeout(context.Background(), tf.timeout)
	responseCh := make(chan error, 1)

	go func() {
		defer close(responseCh)
		defer cancel()
		s, err := tf.self.CreateStream(ctx, receiver, ProtocolInputForward)
		if err != nil {
			responseCh <- errors.Wrap(err, "failed to open stream")
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
		if err := w.Write(req); err != nil {
			_ = s.Reset()
			responseCh <- errors.Errorf("failed to forward transaction, %v", err)
			return
		}
		responseCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("forwarding timeout")
		return ErrTimout
	case err := <-responseCh:
		return err
	}
}

// HandleStream receives incoming transactions from other peers in the network.
func (tf *TxForwarder) HandleStream(s libp2pNetwork.Stream) {
	r := protocol.NewProtoBufReader(s)
	defer func() {
		err := s.Close()
		if err != nil {
			logger.Warning("Failed to close protobuf reader: %v", err)
		}
	}()

	req := &txsystem.Transaction{}
	err := r.Read(req)
	if err != nil {
		logger.Warning("Failed to read the transaction: %v", err)
		return
	}
	tf.receivedMessagesCh <- network.ReceivedMessage{
		From:     s.Conn().RemotePeer(),
		Protocol: ProtocolInputForward,
		Message:  req,
	}
}
