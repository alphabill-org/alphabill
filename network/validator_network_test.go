package network

import (
	"context"
	"crypto"
	"sync/atomic"
	"testing"
	"time"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

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
	defer func() {
		err := h.Close()
		if err != nil {
			t.Fatalf("error closing node %v", err)
		}
	}()

	network, err := NewLibP2PValidatorNetwork(context.Background(), 1, &Peer{host: h}, opts, observability.Default(t))
	require.NoError(t, err)
	require.NotNil(t, network)
}

func TestForwardTransactions_ChangingReceiver(t *testing.T) {
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

	obs := observability.Default(t)
	peer1 := createPeer(t)
	defer func() { require.NoError(t, peer1.Close()) }()
	peer2 := createPeer(t)
	defer func() { require.NoError(t, peer2.Close()) }()

	// peer1 and peer2 are bootstrap peers for peer3
	bootstrapPeers := []peer.AddrInfo{{
		ID:    peer1.ID(),
		Addrs: peer1.host.Addrs(),
	}, {
		ID:    peer2.ID(),
		Addrs: peer2.host.Addrs(),
	}}
	peer3 := createBootstrappedPeer(t, bootstrapPeers, []peer.ID{peer1.ID(), peer2.ID()})
	defer func() { require.NoError(t, peer3.Close()) }()

	network1, err := NewLibP2PValidatorNetwork(context.Background(), 1, peer1, opts, obs)
	require.NoError(t, err)
	require.NotNil(t, network1)

	network2, err := NewLibP2PValidatorNetwork(context.Background(), 1, peer2, opts, obs)
	require.NoError(t, err)
	require.NotNil(t, network2)
	require.NoError(t, peer2.BootstrapConnect(context.Background(), obs.Logger()))

	network3, err := NewLibP2PValidatorNetwork(context.Background(), 1, peer3, opts, obs)
	require.NoError(t, err)
	require.NotNil(t, network3)
	require.NoError(t, peer3.BootstrapConnect(context.Background(), obs.Logger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// peer3 starts forwarding to peer1 and peer2 in a round-robin manner
	go func() error {
		txCount := 0
		network3.ForwardTransactions(ctx, func() peer.ID {
			txCount++
			if txCount%2 == 0 {
				return peer2.ID()
			}
			return peer1.ID()
		})
		return nil
	}()

	// peer1 starts processing
	var peer1TxCount atomic.Int32
	go func() error {
		network1.ProcessTransactions(ctx, func(ctx context.Context, tx *types.TransactionOrder) error {
			peer1TxCount.Add(1)
			return nil
		})
		return nil
	}()

	// peer2 starts processing
	var peer2TxCount atomic.Int32
	go func() error {
		network2.ProcessTransactions(ctx, func(ctx context.Context, tx *types.TransactionOrder) error {
			peer2TxCount.Add(1)
			return nil
		})
		return nil
	}()

	// peer3 has opened bootstrap connections to peer1 and peer2
	require.Eventually(t, func() bool {
		return peer3.Network().Connectedness(peer1.ID()) == network.Connected &&
			peer3.Network().Connectedness(peer2.ID()) == network.Connected
	}, test.WaitDuration, test.WaitTick)

	for i := 0; i < 100; i++ {
		network3.AddTransaction(ctx, transaction.NewTransactionOrder(t))
	}

	require.Eventually(t, func() bool {
		return peer1TxCount.Load() == 50 && peer2TxCount.Load() == 50
	}, test.WaitDuration, test.WaitTick)

	peer1Conns := peer3.Network().ConnsToPeer(peer1.ID())
	peer2Conns := peer3.Network().ConnsToPeer(peer2.ID())
	require.Equal(t, 1, len(peer1Conns))
	require.Equal(t, 1, len(peer2Conns))

	peer1Streams := peer1Conns[0].GetStreams()
	peer2Streams := peer2Conns[0].GetStreams()
	peer1StreamCount := 0
	for _, s := range peer1Streams {
		if s.Protocol() == ProtocolInputForward {
			peer1StreamCount++
		}
	}
	peer2StreamCount := 0
	for _, s := range peer2Streams {
		if s.Protocol() == ProtocolInputForward {
			peer2StreamCount++
		}
	}
	// Verify that streams are closed
	require.LessOrEqual(t, peer1StreamCount, 1)
	require.LessOrEqual(t, peer2StreamCount, 1)
}
