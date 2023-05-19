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

var systemIdentifier = DefaultSystemIdentifier

func TestVDPartition_Ok(t *testing.T) {
	vdPart, err := testpartition.NewPartition(3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := NewTxSystem(systemIdentifier)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{vdPart})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { abNet.Close() })
	tx := createVDTransaction()
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(vdPart, tx), test.WaitDuration, test.WaitTick)

	tx = createVDTransaction()
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(vdPart, tx), test.WaitDuration, test.WaitTick)
}

func TestVDPartition_OnePartitionNodeIsDown(t *testing.T) {
	vdPart, err := testpartition.NewPartition(6, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := NewTxSystem(systemIdentifier)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{vdPart})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { abNet.Close() })
	// killing the leader node can fail the test if all subsequent leader candidates happen to be the killed node within the timeout
	require.ErrorIs(t, vdPart.Nodes[5].Stop(), context.Canceled) // shut down the node

	tx := createVDTransaction()
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContains(vdPart, func(actualTx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, actualTx.UnitId)
	}), test.WaitDuration*2, test.WaitTick)
}

func createVDTransaction() *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:       systemIdentifier,
		UnitId:         hash.Sum256(test.RandomBytes(32)),
		ClientMetadata: &txsystem.ClientMetadata{Timeout: 100},
		OwnerProof:     nil,
	}
}
