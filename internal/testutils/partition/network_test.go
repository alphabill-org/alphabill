package testpartition

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

var systemIdentifier = []byte{1, 2, 4, 1}

func TestNewNetwork_Ok(t *testing.T) {
	counterPartition, err := NewPartition(3,
		func(_ map[string]crypto.Verifier) txsystem.TransactionSystem {
			return &testtxsystem.CounterTxSystem{}
		},
		systemIdentifier)
	require.NoError(t, err)
	abNetwork, err := NewAlphabillPartition([]*NodePartition{counterPartition})
	require.NoError(t, err)
	require.NoError(t, abNetwork.Start())
	defer func() {
		err = abNetwork.Close()
		require.NoError(t, err)
	}()
	require.Len(t, abNetwork.RootPartition.Nodes, 3)
	require.Len(t, abNetwork.NodePartitions, 1)
	cPart, err := abNetwork.GetNodePartition(systemIdentifier)
	require.NoError(t, err)
	require.Len(t, cPart.Nodes, 3)

	tx := testtransaction.NewTransaction(t, testtransaction.WithSystemID(systemIdentifier))
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	require.NoError(t, cPart.SubmitTx(tx))
	require.Eventually(t, BlockchainContainsTx(cPart, tx), test.WaitDuration, test.WaitTick)

	tx = testtransaction.NewTransaction(t, testtransaction.WithSystemID(systemIdentifier))
	fmt.Printf("Broadcasting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = cPart.BroadcastTx(tx)
	require.Eventually(t, BlockchainContainsTx(cPart, tx), test.WaitDuration, test.WaitTick)
}
