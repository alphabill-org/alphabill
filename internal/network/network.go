package network

import (
	"sync"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"

	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"google.golang.org/protobuf/proto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"github.com/libp2p/go-libp2p-core/peer"
)

type (
	protocol interface {
		// ID returns the identifier of the protocol.
		ID() string
	}

	// SendProtocol is a protocol interface to send messages to other nodes.
	SendProtocol interface {
		protocol
		// Send sends the message
		Send(m proto.Message, id peer.ID) error
	}

	// ReceiveProtocol is a protocol interface to receive messages from other nodes.
	ReceiveProtocol interface {
		protocol
		HandleStream(s libp2pNetwork.Stream)
	}

	// OutputMessage represents a message that will be sent to other nodes.
	OutputMessage struct {
		Protocol string        // protocol to use to send the message
		Message  proto.Message // message to send
	}

	// ReceivedMessage represents a message received over the network.
	ReceivedMessage struct {
		From     peer.ID
		Protocol string
		Message  proto.Message
	}
)

type LibP2PNetwork struct {
	self             *Peer
	mtx              sync.Mutex
	receiveProtocols map[string]ReceiveProtocol
	sendProtocols    map[string]SendProtocol
	ReceivedMsgCh    chan ReceivedMessage // messages from LibP2PNetwork to other components.
}

func NewLibP2PNetwork(self *Peer, capacity uint) (*LibP2PNetwork, error) {
	if self == nil {
		return nil, errors.New("peer is nil")
	}
	receivedChannel := make(chan ReceivedMessage, capacity)
	n := &LibP2PNetwork{
		self:             self,
		sendProtocols:    make(map[string]SendProtocol),
		receiveProtocols: make(map[string]ReceiveProtocol),
		ReceivedMsgCh:    receivedChannel,
	}
	return n, nil
}

func (n *LibP2PNetwork) Close() {
	close(n.ReceivedMsgCh)
	for s, _ := range n.receiveProtocols {
		n.self.RemoveProtocolHandler(s)
	}
}

func (n *LibP2PNetwork) ReceivedChannel() <-chan ReceivedMessage {
	return n.ReceivedMsgCh
}

func (n *LibP2PNetwork) RegisterReceiveProtocol(receiveProtocol ReceiveProtocol) error {
	if receiveProtocol == nil {
		return errors.New(errstr.NilArgument)
	}
	n.mtx.Lock()
	defer n.mtx.Unlock()
	if _, f := n.receiveProtocols[receiveProtocol.ID()]; f {
		return errors.Errorf("protocol %v already registered", receiveProtocol.ID())
	}
	n.receiveProtocols[receiveProtocol.ID()] = receiveProtocol
	return nil
}

func (n *LibP2PNetwork) RegisterSendProtocol(sendProtocol SendProtocol) error {
	if sendProtocol == nil {
		return errors.New(errstr.NilArgument)
	}
	n.mtx.Lock()
	defer n.mtx.Unlock()
	if _, f := n.sendProtocols[sendProtocol.ID()]; f {
		return errors.Errorf("protocol %v already registered", sendProtocol.ID())
	}
	n.sendProtocols[sendProtocol.ID()] = sendProtocol
	return nil
}

func (n *LibP2PNetwork) Send(out OutputMessage, receivers []peer.ID) error {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	if len(receivers) == 0 {
		return errors.New("at least one receiver must be present")
	}
	p, f := n.sendProtocols[out.Protocol]
	if !f {
		return errors.Errorf("protocol '%s' is not supported", out.Protocol)
	}
	// TODO start goroutine?
	n.send(p, out.Message, receivers)
	return nil
}

func (n *LibP2PNetwork) send(protocol SendProtocol, m proto.Message, receivers []peer.ID) {
	for _, receiver := range receivers {
		err := protocol.Send(m, receiver)
		if err != nil {
			logger.Warning("Failed to send message to peer %v: %v", receiver, err)
			continue
		}
	}
}
