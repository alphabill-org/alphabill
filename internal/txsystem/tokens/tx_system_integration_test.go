package tokens

import (
	"testing"

	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"

	"github.com/stretchr/testify/require"
)

func TestInitPartition_Ok(t *testing.T) {
	network, err := testpartition.NewNetwork(3, func() txsystem.TransactionSystem {
		system, err := New()
		require.NoError(t, err)
		return system
	}, DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	require.NotNil(t, network)
}
