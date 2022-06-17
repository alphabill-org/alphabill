package testnetwork

import (
	"sync"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

type MockNet struct {
	mutex        sync.Mutex
	MessageCh    chan network.ReceivedMessage
	sentMessages map[string][]PeerMessage
}

type PeerMessage struct {
	peer.ID
	network.OutputMessage
}

func NewMockNetwork() *MockNet {
	return &MockNet{
		MessageCh:    make(chan network.ReceivedMessage, 100),
		sentMessages: make(map[string][]PeerMessage),
	}
}

func (m *MockNet) Send(msg network.OutputMessage, receivers []peer.ID) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	messages := m.sentMessages[msg.Protocol]
	for _, r := range receivers {
		messages = append(messages, PeerMessage{
			ID:            r,
			OutputMessage: msg,
		})
	}
	m.sentMessages[msg.Protocol] = messages
	return nil
}

func (m *MockNet) SentMessages(protocol string) []PeerMessage {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.sentMessages[protocol]
}

func (m *MockNet) ResetSentMessages(protocol string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.sentMessages[protocol] = []PeerMessage{}
}

func (m *MockNet) Receive(msg network.ReceivedMessage) {
	m.MessageCh <- msg
}

func (m *MockNet) ReceivedChannel() <-chan network.ReceivedMessage {
	return m.MessageCh
}
