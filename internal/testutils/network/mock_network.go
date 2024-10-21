package testnetwork

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/network/protocol/replication"
	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/alphabill-org/alphabill/txbuffer"
)

type MockNet struct {
	mutex        sync.Mutex
	err          error
	MessageCh    chan any
	txBuffer     *txbuffer.TxBuffer
	sentMessages map[string][]PeerMessage
	protocols    map[reflect.Type]string
}

type PeerMessage struct {
	peer.ID
	Message any
}

func NewMockNetwork(t *testing.T) *MockNet {
	obs := observability.Default(t)
	txBuffer, err := txbuffer.New(100, crypto.SHA256, obs)
	require.NoError(t, err)

	mn := &MockNet{
		MessageCh:    make(chan any, 100),
		txBuffer:     txBuffer,
		sentMessages: make(map[string][]PeerMessage),
		protocols:    make(map[reflect.Type]string),
	}
	err = mn.registerSendProtocols([]msgProtocol{
		{protocolID: network.ProtocolBlockProposal, msgStruct: blockproposal.BlockProposal{}},
		{protocolID: network.ProtocolBlockCertification, msgStruct: certification.BlockCertificationRequest{}},
		{protocolID: network.ProtocolInputForward, msgStruct: types.TransactionOrder{}},
		{protocolID: network.ProtocolLedgerReplicationReq, msgStruct: replication.LedgerReplicationRequest{}},
		{protocolID: network.ProtocolLedgerReplicationResp, msgStruct: replication.LedgerReplicationResponse{}},
		{protocolID: network.ProtocolHandshake, msgStruct: handshake.Handshake{}},
		{protocolID: network.ProtocolUnicityCertificates, msgStruct: types.UnicityCertificate{}},
	})
	if err != nil {
		panic(fmt.Errorf("failed to register protocols: %w", err))
	}
	return mn
}

func NewRootMockNetwork() *MockNet {
	mn := &MockNet{
		MessageCh:    make(chan any, 100),
		sentMessages: make(map[string][]PeerMessage),
		protocols:    make(map[reflect.Type]string),
	}
	err := mn.registerSendProtocols([]msgProtocol{
		{protocolID: network.ProtocolRootIrChangeReq, msgStruct: abtypes.IRChangeReq{}},
		{protocolID: network.ProtocolRootProposal, msgStruct: abdrc.ProposalMsg{}},
		{protocolID: network.ProtocolRootVote, msgStruct: abdrc.VoteMsg{}},
		{protocolID: network.ProtocolRootTimeout, msgStruct: abdrc.TimeoutMsg{}},
		{protocolID: network.ProtocolRootStateReq, msgStruct: abdrc.StateRequestMsg{}},
		{protocolID: network.ProtocolRootStateResp, msgStruct: abdrc.StateMsg{}},
	})
	if err != nil {
		panic(fmt.Errorf("failed to register protocols: %w", err))
	}
	return mn
}

func (m *MockNet) Send(ctx context.Context, msg any, receivers ...peer.ID) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	// mock send error
	if m.err != nil {
		return m.err
	}

	protocolID, ok := m.protocols[reflect.TypeOf(msg)]
	if !ok {
		return fmt.Errorf("no protocol registered for data type %T", msg)
	}
	messages := m.sentMessages[protocolID]
	for _, r := range receivers {
		messages = append(messages, PeerMessage{
			ID:      r,
			Message: msg,
		})
	}
	m.sentMessages[protocolID] = messages
	return nil
}

func (m *MockNet) SetErrorState(err error) {
	m.err = err
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

func (m *MockNet) Receive(msg any) {
	m.MessageCh <- msg
}

func (m *MockNet) ReceivedChannel() <-chan any {
	return m.MessageCh
}

func (m *MockNet) AddTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	if m.txBuffer != nil {
		return m.txBuffer.Add(ctx, tx)
	}
	return nil, nil
}

func (m *MockNet) ProcessTransactions(ctx context.Context, txProcessor network.TxProcessor) {
	for {
		tx, err := m.txBuffer.Remove(ctx)
		if err != nil {
			return
		}
		if err := txProcessor(ctx, tx); err != nil {
			continue
		}
	}
}

func (m *MockNet) ForwardTransactions(ctx context.Context, receiverFunc network.TxReceiver) {
}

func (m *MockNet) PublishBlock(ctx context.Context, block *types.Block) error {
	return nil
}

type msgProtocol struct {
	msgStruct  any
	protocolID string
}

func (m *MockNet) registerSendProtocols(protocols []msgProtocol) error {
	for _, pd := range protocols {
		if err := m.registerSendProtocol(pd.msgStruct, pd.protocolID); err != nil {
			return fmt.Errorf("registering send protocol %q: %w", pd.protocolID, err)
		}
	}
	return nil
}

func (m *MockNet) registerSendProtocol(msgStruct any, protocolID string) error {
	if protocolID == "" {
		return errors.New("protocol ID must be assigned")
	}

	typ := reflect.TypeOf(msgStruct)
	if typ == nil {
		return errors.New("message data type must be assigned")
	}
	if typ.Kind() != reflect.Struct {
		return fmt.Errorf("message data type must be struct, got %s %v", typ.Kind(), msgStruct)
	}

	if pid, ok := m.protocols[typ]; ok {
		return fmt.Errorf("data type %s has been already registered for protocol %s", typ, pid)
	}

	m.protocols[typ] = protocolID
	m.protocols[reflect.PointerTo(typ)] = protocolID
	return nil
}
