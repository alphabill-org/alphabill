package verifiable_data

import (
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testpartition "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

var systemIdentifier = []byte{0, 0, 0, 1}

func TestVDPartition_Ok(t *testing.T) {
	network, err := testpartition.NewNetwork(3, func() txsystem.TransactionSystem {
		system, err := New(systemIdentifier)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)

	tx := createVDTransaction()
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	tx = createVDTransaction()
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}

func TestVDPartition_OnePartitionNodeIsDown(t *testing.T) {
	network, err := testpartition.NewNetwork(3, func() txsystem.TransactionSystem {
		system, err := New(systemIdentifier)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)
	// TODO Killing Node[1] fails the test as #1 is a deterministic leader
	network.Nodes[2].Close() // shut down the node

	tx := createVDTransaction()
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}

func createVDTransaction() *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:   systemIdentifier,
		UnitId:     hash.Sum256(test.RandomBytes(32)),
		Timeout:    100,
		OwnerProof: nil,
	}
}
