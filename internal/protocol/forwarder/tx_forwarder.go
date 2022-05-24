package forwarder

import (
	"context"
	"io/ioutil"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

var (
	ErrPeerIsNil      = errors.New("peer is nil")
	ErrTxHandlerIsNil = errors.New("tx handler is nil")
	ErrTimout         = errors.New("forwarding timeout")
)

const (
	// ProtocolIdTxForwarder is the protocol.ID of the Alphabill transaction forwarding protocol.
	ProtocolIdTxForwarder    = "/ab/pc1-I/0.0.1"
	DefaultForwardingTimeout = 700 * time.Second
)

type (

	// TxForwarder sends transactions to the expected leader.
	TxForwarder struct {
		self               *network.Peer
		transactionHandler TxHandler
		timeout            time.Duration
	}

	TxHandler func(tx *txsystem.Transaction)
)

func New(self *network.Peer, timeout time.Duration, transactionHandler func(tx *txsystem.Transaction)) (*TxForwarder, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if transactionHandler == nil {
		return nil, ErrTxHandlerIsNil
	}
	t := &TxForwarder{self: self, transactionHandler: transactionHandler, timeout: timeout}
	self.RegisterProtocolHandler(ProtocolIdTxForwarder, t.handleStream)
	return t, nil
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
		s, err := tf.self.CreateStream(ctx, receiver, ProtocolIdTxForwarder)
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

		response, err := ioutil.ReadAll(s)
		if err != nil {
			responseCh <- errors.Errorf("failed to read response, %v", err)
			return
		}
		if string(response) != "received" {
			responseCh <- errors.Errorf("invalid response from peer %v. response: %v", receiver, string(response))
			return
		}
		logger.Debug("forwarded tx to peer %v", receiver)
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

// handleStream receives incoming transactions from other peers in the network.
func (tf *TxForwarder) handleStream(s libp2pNetwork.Stream) {
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
	tf.transactionHandler(req)
	_, err = s.Write([]byte("received"))
	if err != nil {
		logger.Warning("Failed to write response: %v", err)
	}
}
