package testnetwork

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

type MockNet struct {
	MessageCh        chan network.ReceivedMessage
	ReceivedMessages map[string][]PeerMessage // protocol => message
	SentMessages     map[string][]PeerMessage
}

type PeerMessage struct {
	peer.ID
	network.OutputMessage
}

func NewMockNetwork() *MockNet {
	return &MockNet{
		ReceivedMessages: make(map[string][]PeerMessage),
		MessageCh:        make(chan network.ReceivedMessage, 100),
		SentMessages:     make(map[string][]PeerMessage),
	}
}

func (m *MockNet) Send(msg network.OutputMessage, receivers []peer.ID) error {
	messages := m.SentMessages[msg.Protocol]
	for _, r := range receivers {
		messages = append(messages, PeerMessage{
			ID:            r,
			OutputMessage: msg,
		})
	}
	m.SentMessages[msg.Protocol] = messages
	return nil
}

func (m *MockNet) Receive(msg network.ReceivedMessage) {
	m.MessageCh <- msg
}

func (m *MockNet) ReceivedChannel() <-chan network.ReceivedMessage {
	return m.MessageCh
}
