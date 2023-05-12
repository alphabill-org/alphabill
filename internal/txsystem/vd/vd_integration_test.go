package vd

import (
	"bytes"
	"context"
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
		system, err := NewTxSystem(systemIdentifier)
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
		system, err := NewTxSystem(systemIdentifier)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)
	// Killing the leader node fails the test
	require.ErrorIs(t, network.Nodes[2].Stop(), context.Canceled) // shut down the node

	tx := createVDTransaction()
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContains(network, func(actualTx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, actualTx.UnitId)
	}), test.WaitDuration, test.WaitTick)
}

func createVDTransaction() *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:       systemIdentifier,
		UnitId:         hash.Sum256(test.RandomBytes(32)),
		ClientMetadata: &txsystem.ClientMetadata{Timeout: 100},
		OwnerProof:     nil,
	}
}
