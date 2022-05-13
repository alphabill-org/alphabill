package verifiable_data

import (
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"github.com/stretchr/testify/require"

	testpartition "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
)

var systemIdentifier = []byte{0, 0, 0, 2}

func TestVDPartition_Ok(t *testing.T) {
	network, err := testpartition.NewNetwork(3, func() txsystem.TransactionSystem {
		system, err := New()
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)

	tx := &transaction.Transaction{
		SystemId:   systemIdentifier,
		UnitId:     hash.Sum256(test.RandomBytes(32)),
		Timeout:    100,
		OwnerProof: nil,
	}
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	tx = &transaction.Transaction{
		SystemId:   systemIdentifier,
		UnitId:     hash.Sum256(test.RandomBytes(32)),
		Timeout:    100,
		OwnerProof: nil,
	}
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}
