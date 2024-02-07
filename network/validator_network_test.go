package network

import (
	"context"
	"crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/types"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/config"
	"github.com/stretchr/testify/require"
)

type MockTxProcessor struct {
	received []*types.TransactionOrder
}

func (m *MockTxProcessor) ProcessTransactions(ctx context.Context, tx *types.TransactionOrder) error {
	m.received = append(m.received, tx)
	return nil
}

func TestNewLibP2PValidatorNetwork(t *testing.T) {
	opts := ValidatorNetworkOptions{
		ReceivedChannelCapacity:          1000,
		TxBufferSize:                     1000,
		TxBufferHashAlgorithm:            crypto.SHA256,
		BlockCertificationTimeout:        300 * time.Millisecond,
		BlockProposalTimeout:             300 * time.Millisecond,
		LedgerReplicationRequestTimeout:  300 * time.Millisecond,
		LedgerReplicationResponseTimeout: 300 * time.Millisecond,
		HandshakeTimeout:                 300 * time.Millisecond,
	}

	h, err := libp2p.New([]config.Option{
		libp2p.ListenAddrStrings(defaultAddress),
	}...)
	require.NoError(t, err)
	network, err := NewLibP2PValidatorNetwork(&Peer{host: h}, opts, observability.Default(t))
	require.NoError(t, err)
	require.NotNil(t, network)
}

func TestValidatorNetwork_ProcessTransactions(t *testing.T) {
	opts := ValidatorNetworkOptions{
		ReceivedChannelCapacity:          1000,
		TxBufferSize:                     1000,
		TxBufferHashAlgorithm:            crypto.SHA256,
		BlockCertificationTimeout:        300 * time.Millisecond,
		BlockProposalTimeout:             300 * time.Millisecond,
		LedgerReplicationRequestTimeout:  300 * time.Millisecond,
		LedgerReplicationResponseTimeout: 300 * time.Millisecond,
		HandshakeTimeout:                 300 * time.Millisecond,
	}

	h, err := libp2p.New([]config.Option{
		libp2p.ListenAddrStrings(defaultAddress),
	}...)
	require.NoError(t, err)
	network, err := NewLibP2PValidatorNetwork(&Peer{host: h}, opts, observability.Default(t))
	require.NoError(t, err)
	require.NotNil(t, network)

	mockTxProcessor := &MockTxProcessor{received: make([]*types.TransactionOrder, 1)}
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() {
		network.ProcessTransactions(ctx, mockTxProcessor.ProcessTransactions)
	}()

	tx := &types.TransactionOrder{}
	_, err = network.AddTransaction(context.Background(), tx)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(mockTxProcessor.received) > 0 }, 3*time.Second, 200*time.Millisecond)
}

func TestValidatorNetwork_ForwardTransaction(t *testing.T) {
	opts := ValidatorNetworkOptions{
		ReceivedChannelCapacity:          1000,
		TxBufferSize:                     1000,
		TxBufferHashAlgorithm:            crypto.SHA256,
		BlockCertificationTimeout:        300 * time.Millisecond,
		BlockProposalTimeout:             300 * time.Millisecond,
		LedgerReplicationRequestTimeout:  300 * time.Millisecond,
		LedgerReplicationResponseTimeout: 300 * time.Millisecond,
		HandshakeTimeout:                 300 * time.Millisecond,
	}

	h, err := libp2p.New([]config.Option{
		libp2p.ListenAddrStrings(defaultAddress),
	}...)
	require.NoError(t, err)
	network, err := NewLibP2PValidatorNetwork(&Peer{host: h}, opts, observability.Default(t))
	require.NoError(t, err)
	require.NotNil(t, network)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	tx := &types.TransactionOrder{}
	_, err = network.AddTransaction(ctx, tx)
	require.NoError(t, err)

	go func() {
		network.ForwardTransactions(ctx, "receiver_id")
	}()

	require.Eventually(t, func() bool {
		return ctx.Err() == nil
	}, 3*time.Second, 200*time.Millisecond)
}
