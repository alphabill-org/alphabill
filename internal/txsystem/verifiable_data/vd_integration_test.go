package verifiable_data

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

var systemIdentifier = []byte{0, 0, 0, 1}

func TestVDPartition_Ok(t *testing.T) {
	network, err := testpartition.NewNetwork(3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := New(systemIdentifier)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)

	tx := createVDTransaction()
	require.NoError(t, network.SubmitTx(tx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	tx = createVDTransaction()
	require.NoError(t, network.SubmitTx(tx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}

func createVDTransaction() *types.TransactionOrder {
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           txType,
			SystemID:       systemIdentifier,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			ClientMetadata: &types.ClientMetadata{Timeout: 100},
			Attributes:     []byte{0xf6}, // nil
		},
		OwnerProof: nil,
	}
}
