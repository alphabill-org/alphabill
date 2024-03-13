package network

import (
	"crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/config"
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

	network, err := NewLibP2PValidatorNetwork(&Peer{host: h}, opts, observability.Default(t))
	require.NoError(t, err)
	require.NotNil(t, network)
}
