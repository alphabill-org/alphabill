package network

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/config"
	"github.com/stretchr/testify/require"
)

func TestNewLibP2RootConsensusNetwork(t *testing.T) {
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
	peer := &Peer{host: h}

	capacity := uint(1000)
	sendTimeout := 300 * time.Millisecond
	result, err := NewLibP2RootConsensusNetwork(peer, capacity, sendTimeout, observability.Default(t))

	require.NoError(t, err)
	require.Equal(t, h.ID(), result.self.ID())
}
