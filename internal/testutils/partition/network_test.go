package testpartition

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func TestNewNetwork_Ok(t *testing.T) {
	const systemIdentifier types.SystemID = 0x01020401
	genesisState := state.NewEmptyState()
	counterPartition, err := NewPartition(t, 3,
		func(_ map[string]crypto.Verifier) txsystem.TransactionSystem {
			txs := &testtxsystem.CounterTxSystem{}
			txs.Commit(genesisState.CommittedUC())
			return txs
		},
		systemIdentifier, genesisState)
	require.NoError(t, err)
	abNetwork, err := NewMultiRootAlphabillPartition(3, []*NodePartition{counterPartition})
	require.NoError(t, err)
	require.NoError(t, abNetwork.Start(t))
	defer abNetwork.WaitClose(t)

	require.Len(t, abNetwork.RootPartition.Nodes, 3)
	require.Len(t, abNetwork.NodePartitions, 1)
	cPart, err := abNetwork.GetNodePartition(systemIdentifier)
	require.NoError(t, err)
	require.Len(t, cPart.Nodes, 3)
	time.Sleep(time.Second * 5)
	tx := testtransaction.NewTransactionOrder(t, testtransaction.WithSystemID(systemIdentifier))
	require.NoError(t, cPart.SubmitTx(tx))
	require.Eventually(t, BlockchainContainsTx(cPart, tx), test.WaitDuration, test.WaitTick)

	tx = testtransaction.NewTransactionOrder(t, testtransaction.WithSystemID(systemIdentifier))
	require.NoError(t, cPart.BroadcastTx(tx))

	require.Eventually(t, BlockchainContainsTx(cPart, tx), test.WaitDuration, test.WaitTick)
}
