package network

import (
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	drctypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

var DefaultValidatorNetOptions = ValidatorNetOptions{
	ResponseChannelCapacity:          1000,
	ForwarderTimeout:                 300 * time.Millisecond,
	BlockCertificationTimeout:        300 * time.Millisecond,
	BlockProposalTimeout:             300 * time.Millisecond,
	LedgerReplicationRequestTimeout:  300 * time.Millisecond,
	LedgerReplicationResponseTimeout: 300 * time.Millisecond,
	HandshakeTimeout:                 300 * time.Millisecond,
}

type (

	// OutputMessage represents a message that will be sent to other nodes.
	OutputMessage struct {
		Protocol string // protocol to use to send the message
		Message  any    // message to send
	}

	// ReceivedMessage represents a message received over the network.
	ReceivedMessage struct {
		From     peer.ID
		Protocol string
		Message  any
	}

	ValidatorNetOptions struct {
		ResponseChannelCapacity          uint
		ForwarderTimeout                 time.Duration
		BlockCertificationTimeout        time.Duration
		BlockProposalTimeout             time.Duration
		LedgerReplicationRequestTimeout  time.Duration
		LedgerReplicationResponseTimeout time.Duration
		HandshakeTimeout                 time.Duration
	}

	sendProtocolDescription struct {
		protocolID string
		timeout    time.Duration
	}

	receiveProtocolDescription struct {
		protocolID string
		typeFn     TypeFunc[any]
	}
)

type LibP2PNetwork struct {
	self             *Peer
	receiveProtocols map[string]*ReceiveProtocol[any]
	sendProtocols    map[string]*SendProtocol
	ReceivedMsgCh    chan ReceivedMessage // messages from LibP2PNetwork to other components.
}

// NewLibP2PNetwork creates a new libP2P network without protocols.
func NewLibP2PNetwork(self *Peer, capacity uint) (*LibP2PNetwork, error) {
	if self == nil {
		return nil, errors.New("peer is nil")
	}
	receivedChannel := make(chan ReceivedMessage, capacity)
	n := &LibP2PNetwork{
		self:             self,
		sendProtocols:    make(map[string]*SendProtocol),
		receiveProtocols: make(map[string]*ReceiveProtocol[any]),
		ReceivedMsgCh:    receivedChannel,
	}
	return n, nil
}

// NewLibP2PValidatorNetwork creates a new libp2p for a validator.
func NewLibP2PValidatorNetwork(self *Peer, opts ValidatorNetOptions) (*LibP2PNetwork, error) {
	n, err := NewLibP2PNetwork(self, opts.ResponseChannelCapacity)
	if err != nil {
		return nil, err
	}
	sendProtocolDescriptions := []sendProtocolDescription{
		{protocolID: ProtocolBlockProposal, timeout: opts.BlockProposalTimeout},
		{protocolID: ProtocolBlockCertification, timeout: opts.BlockCertificationTimeout},
		{protocolID: ProtocolInputForward, timeout: opts.ForwarderTimeout},
		{protocolID: ProtocolLedgerReplicationReq, timeout: opts.LedgerReplicationRequestTimeout},
		{protocolID: ProtocolLedgerReplicationResp, timeout: opts.LedgerReplicationResponseTimeout},
		{protocolID: ProtocolHandshake, timeout: opts.HandshakeTimeout},
	}
	err = initSendProtocols(self, sendProtocolDescriptions, n)
	if err != nil {
		return nil, err
	}
	receiveProtocolDescriptions := []receiveProtocolDescription{
		{
			protocolID: ProtocolBlockProposal,
			typeFn:     func() any { return &blockproposal.BlockProposal{} },
		},
		{
			protocolID: ProtocolInputForward,
			typeFn:     func() any { return &types.TransactionOrder{} },
		},
		{
			protocolID: ProtocolUnicityCertificates,
			typeFn:     func() any { return &types.UnicityCertificate{} },
		},
		{
			protocolID: ProtocolLedgerReplicationReq,
			typeFn:     func() any { return &replication.LedgerReplicationRequest{} },
		},
		{
			protocolID: ProtocolLedgerReplicationResp,
			typeFn:     func() any { return &replication.LedgerReplicationResponse{} },
		},
	}
	err = initReceiveProtocols(self, n, receiveProtocolDescriptions)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func NewLibP2PRootChainNetwork(self *Peer, capacity uint, sendCertificateTimeout time.Duration) (*LibP2PNetwork, error) {
	n, err := NewLibP2PNetwork(self, capacity)
	if err != nil {
		return nil, err
	}
	sendProtocolDescriptions := []sendProtocolDescription{
		{protocolID: ProtocolUnicityCertificates, timeout: sendCertificateTimeout},
	}
	err = initSendProtocols(self, sendProtocolDescriptions, n)
	if err != nil {
		return nil, err
	}
	receiveProtocolDescriptions := []receiveProtocolDescription{
		{
			protocolID: ProtocolBlockCertification,
			typeFn:     func() any { return &certification.BlockCertificationRequest{} },
		},
		{
			protocolID: ProtocolHandshake,
			typeFn:     func() any { return &handshake.Handshake{} },
		},
	}
	err = initReceiveProtocols(self, n, receiveProtocolDescriptions)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func NewLibP2RootConsensusNetwork(self *Peer, capacity uint, sendTimeout time.Duration) (*LibP2PNetwork, error) {
	n, err := NewLibP2PNetwork(self, capacity)
	if err != nil {
		return nil, err
	}
	sendProtocolDescriptions := []sendProtocolDescription{
		{protocolID: ProtocolRootIrChangeReq, timeout: sendTimeout},
		{protocolID: ProtocolRootProposal, timeout: sendTimeout},
		{protocolID: ProtocolRootVote, timeout: sendTimeout},
		{protocolID: ProtocolRootTimeout, timeout: sendTimeout},
		{protocolID: ProtocolRootStateReq, timeout: sendTimeout},
		{protocolID: ProtocolRootStateResp, timeout: sendTimeout},
	}
	err = initSendProtocols(self, sendProtocolDescriptions, n)
	if err != nil {
		return nil, err
	}
	receiveProtocolDescriptions := []receiveProtocolDescription{
		{
			protocolID: ProtocolRootIrChangeReq,
			typeFn:     func() any { return &drctypes.IRChangeReq{} },
		},
		{
			protocolID: ProtocolRootProposal,
			typeFn:     func() any { return &abdrc.ProposalMsg{} },
		},
		{
			protocolID: ProtocolRootVote,
			typeFn:     func() any { return &abdrc.VoteMsg{} },
		},
		{
			protocolID: ProtocolRootTimeout,
			typeFn:     func() any { return &abdrc.TimeoutMsg{} },
		},
		{
			protocolID: ProtocolRootStateReq,
			typeFn:     func() any { return &abdrc.GetStateMsg{} },
		},
		{
			protocolID: ProtocolRootStateResp,
			typeFn:     func() any { return &abdrc.StateMsg{} },
		},
	}
	err = initReceiveProtocols(self, n, receiveProtocolDescriptions)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (n *LibP2PNetwork) Close() {
	close(n.ReceivedMsgCh)
	for s := range n.receiveProtocols {
		n.self.RemoveProtocolHandler(s)
	}
}

func (n *LibP2PNetwork) ReceivedChannel() <-chan ReceivedMessage {
	return n.ReceivedMsgCh
}

func (n *LibP2PNetwork) registerReceiveProtocol(receiveProtocol *ReceiveProtocol[any]) error {
	if receiveProtocol == nil {
		return errors.New("receiver protocol is nil")
	}
	if _, f := n.receiveProtocols[receiveProtocol.ID()]; f {
		return fmt.Errorf("protocol %v already registered", receiveProtocol.ID())
	}
	n.receiveProtocols[receiveProtocol.ID()] = receiveProtocol
	return nil
}

func (n *LibP2PNetwork) registerSendProtocol(sendProtocol *SendProtocol) error {
	if sendProtocol == nil {
		return errors.New("send protocol is nil")
	}
	if _, f := n.sendProtocols[sendProtocol.ID()]; f {
		return fmt.Errorf("protocol %v already registered", sendProtocol.ID())
	}
	n.sendProtocols[sendProtocol.ID()] = sendProtocol
	return nil
}

func (n *LibP2PNetwork) Broadcast(msg OutputMessage) error {
	// use n.self.host.Peerstore().Peers()? current code used conf so...
	return n.Send(msg, n.self.conf.Validators)
}

func (n *LibP2PNetwork) Send(out OutputMessage, receivers []peer.ID) error {
	if len(receivers) == 0 {
		return nil // no one to send message in single-node partition
	}
	p, f := n.sendProtocols[out.Protocol]
	if !f {
		return fmt.Errorf("protocol '%s' is not supported", out.Protocol)
	}
	go n.send(p, out.Message, receivers)
	return nil
}

func (n *LibP2PNetwork) send(protocol *SendProtocol, m any, receivers []peer.ID) {
	for _, receiver := range receivers {
		// loop-back for self messages
		if receiver == n.self.ID() {
			if _, f := n.receiveProtocols[protocol.protocolID]; !f {
				logger.Warning("Loop-back failed message receive protocol %v not supported", protocol.protocolID)
				continue
			}
			n.ReceivedMsgCh <- ReceivedMessage{
				From:     n.self.ID(),
				Protocol: protocol.protocolID,
				Message:  m,
			}
			continue
		}
		err := protocol.Send(m, receiver)
		if err != nil {
			logger.Warning("Send error, message %v receiver: %v sender: %v, %v",
				protocol.protocolID, receiver, n.self.ID(), err)
			continue
		}
	}
}

func initReceiveProtocols(self *Peer, n *LibP2PNetwork, receiveProtocolDescriptions []receiveProtocolDescription) error {
	for _, d := range receiveProtocolDescriptions {
		err := initReceiveProtocol(self, d.protocolID, d.typeFn, n)
		if err != nil {
			return fmt.Errorf("receive protocol %v init failed, %w", d.protocolID, err)
		}
	}
	return nil
}

func initReceiveProtocol(self *Peer, protocolID string, typeFn TypeFunc[any], n *LibP2PNetwork) error {
	p, err := NewReceiverProtocol(self, protocolID, n.ReceivedMsgCh, typeFn)
	if err != nil {
		return fmt.Errorf("new receive protocol error, %w", err)
	}
	err = n.registerReceiveProtocol(p)
	if err != nil {
		return fmt.Errorf("failed to register receive protocol, %w", err)
	}
	return nil
}

func initSendProtocols(self *Peer, sendProtocolDescriptions []sendProtocolDescription, n *LibP2PNetwork) error {
	for _, pd := range sendProtocolDescriptions {
		err := initSendProtocol(pd.protocolID, self, pd.timeout, n)
		if err != nil {
			return fmt.Errorf("send protocol %v init failed, %w", pd.protocolID, err)
		}
	}
	return nil
}

func initSendProtocol(protocolID string, peer *Peer, timeout time.Duration, n *LibP2PNetwork) error {
	p, err := NewSendProtocol(peer, protocolID, timeout)
	if err != nil {
		return fmt.Errorf("new send protocol error, %w", err)
	}
	if err = n.registerSendProtocol(p); err != nil {
		return fmt.Errorf("failed to register send protocol, %w", err)
	}
	return nil
}
