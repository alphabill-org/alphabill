package verifiable_data

import (
	"fmt"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testpartition "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

var systemIdentifier = []byte{0, 0, 0, 2}

func TestVDPartition_Ok(t *testing.T) {
	network, err := testpartition.NewNetwork(3, func() txsystem.TransactionSystem {
		system, err := New()
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)

	tx := &txsystem.Transaction{
		SystemId:   systemIdentifier,
		UnitId:     hash.Sum256(test.RandomBytes(32)),
		Timeout:    100,
		OwnerProof: nil,
	}
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), 3*test.WaitDuration, test.WaitTick)

	tx = &txsystem.Transaction{
		SystemId:   systemIdentifier,
		UnitId:     hash.Sum256(test.RandomBytes(32)),
		Timeout:    100,
		OwnerProof: nil,
	}
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), 3*test.WaitDuration, test.WaitTick)
}
