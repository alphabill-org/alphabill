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
	vdPart, err := testpartition.NewPartition(3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := New(systemIdentifier)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{vdPart})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { abNet.Close() })
	tx := createVDTransaction()
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(vdPart, tx), test.WaitDuration, test.WaitTick)

	tx = createVDTransaction()
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(vdPart, tx), test.WaitDuration, test.WaitTick)
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
