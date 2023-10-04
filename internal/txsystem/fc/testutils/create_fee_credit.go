package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

// CreateFeeCredit creates fee credit to be able to spend initial bill
func CreateFeeCredit(t *testing.T, initialBillID, fcrID types.UnitID, fcrAmount uint64, network *testpartition.AlphabillNetwork) *types.TransactionOrder {
	// send transferFC
	transferFC := NewTransferFC(t,
		NewTransferFCAttr(
			WithBacklink(nil),
			WithAmount(fcrAmount),
			WithTargetRecordID(fcrID),
		),
		testtransaction.WithUnitId(initialBillID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(transactions.PayloadTypeTransferFeeCredit),
	)
	moneyPartition, err := network.GetNodePartition([]byte{0, 0, 0, 0})
	require.NoError(t, err)
	err = moneyPartition.SubmitTx(transferFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, transferFC), test.WaitDuration, test.WaitTick)

	// send addFC
	_, transferFCProof, transferFCRecord, err := moneyPartition.GetTxProof(transferFC)
	require.NoError(t, err)
	addFC := NewAddFC(t, network.RootPartition.Nodes[0].RootSigner,
		NewAddFCAttr(t, network.RootPartition.Nodes[0].RootSigner,
			WithTransferFCTx(transferFCRecord),
			WithTransferFCProof(transferFCProof),
			WithFCOwnerCondition(script.PredicateAlwaysTrue()),
		),
		testtransaction.WithUnitId(fcrID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(transactions.PayloadTypeAddFeeCredit),
	)
	err = moneyPartition.SubmitTx(addFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, addFC), test.WaitDuration, test.WaitTick)
	return transferFCRecord.TransactionOrder
}
