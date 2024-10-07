package testpartition

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestNewNetwork_Ok(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		NetworkIdentifier: 5,
		SystemIdentifier:  0x01020401,
		TypeIdLen:         8,
		UnitIdLen:         256,
		T2Timeout:         3 * time.Second,
	}
	genesisState := state.NewEmptyState()
	counterPartition, err := NewPartition(t, 3,
		func(_ types.RootTrustBase) txsystem.TransactionSystem {
			txs := &testtxsystem.CounterTxSystem{}
			require.NoError(t, txs.Commit(genesisState.CommittedUC()))
			return txs
		},
		pdr, genesisState)
	require.NoError(t, err)
	abNetwork, err := NewMultiRootAlphabillPartition(3, []*NodePartition{counterPartition})
	require.NoError(t, err)
	require.NoError(t, abNetwork.Start(t))
	defer abNetwork.WaitClose(t)

	require.Len(t, abNetwork.RootPartition.Nodes, 3)
	require.Len(t, abNetwork.NodePartitions, 1)
	cPart, err := abNetwork.GetNodePartition(pdr.SystemIdentifier)
	require.NoError(t, err)
	require.Len(t, cPart.Nodes, 3)
	require.Eventually(t, PartitionInitReady(t, cPart), test.WaitDuration, test.WaitTick)
	tx := testtransaction.NewTransactionOrder(t, testtransaction.WithSystemID(pdr.SystemIdentifier))
	require.NoError(t, cPart.SubmitTx(tx))
	test.TryTilCountIs(t, BlockchainContainsTx(cPart, tx), 40, test.WaitTick)

	tx = testtransaction.NewTransactionOrder(t, testtransaction.WithSystemID(pdr.SystemIdentifier))
	require.NoError(t, cPart.BroadcastTx(tx))

	test.TryTilCountIs(t, BlockchainContainsTx(cPart, tx), 40, test.WaitTick)
}

func TestNewNetwork_StandaloneBootstrapNodes(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		NetworkIdentifier: 5,
		SystemIdentifier:  0x01020401,
		TypeIdLen:         8,
		UnitIdLen:         256,
		T2Timeout:         3 * time.Second,
	}
	genesisState := state.NewEmptyState()
	counterPartition, err := NewPartition(t, 3,
		func(_ types.RootTrustBase) txsystem.TransactionSystem {
			txs := &testtxsystem.CounterTxSystem{}
			require.NoError(t, txs.Commit(genesisState.CommittedUC()))
			return txs
		},
		pdr, genesisState)
	require.NoError(t, err)
	abNetwork, err := NewMultiRootAlphabillPartition(3, []*NodePartition{counterPartition})
	require.NoError(t, err)
	require.NoError(t, abNetwork.StartWithStandAloneBootstrapNodes(t))
	defer abNetwork.WaitClose(t)

	require.Len(t, abNetwork.RootPartition.Nodes, 3)
	require.Len(t, abNetwork.NodePartitions, 1)
	cPart, err := abNetwork.GetNodePartition(pdr.SystemIdentifier)
	require.NoError(t, err)
	require.Len(t, cPart.Nodes, 3)
	test.TryTilCountIs(t, PartitionInitReady(t, cPart), 40, test.WaitTick)
	tx := testtransaction.NewTransactionOrder(t, testtransaction.WithSystemID(pdr.SystemIdentifier))
	require.NoError(t, cPart.SubmitTx(tx))
	test.TryTilCountIs(t, BlockchainContainsTx(cPart, tx), 40, test.WaitTick)

	tx = testtransaction.NewTransactionOrder(t, testtransaction.WithSystemID(pdr.SystemIdentifier))
	require.NoError(t, cPart.BroadcastTx(tx))

	test.TryTilCountIs(t, BlockchainContainsTx(cPart, tx), 40, test.WaitTick)
}
