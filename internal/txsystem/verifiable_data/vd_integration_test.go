package verifiable_data

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
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
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	tx = createVDTransaction()
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}

func TestVDPartition_OnePartitionNodeIsDown(t *testing.T) {
	network, err := testpartition.NewNetwork(3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := New(systemIdentifier)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)
	// TODO Killing Node[2] fails the test as #2 is a deterministic leader
	network.Nodes[1].Close() // shut down the node

	tx := createVDTransaction()
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
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
