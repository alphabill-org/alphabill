package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill/api/predicates/templates"
	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	test "github.com/alphabill-org/alphabill/validator/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/validator/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/validator/internal/testutils/transaction"
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
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(transactions.PayloadTypeTransferFeeCredit),
	)
	moneyPartition, err := network.GetNodePartition([]byte{0, 0, 0, 0})
	require.NoError(t, err)
	require.NoError(t, moneyPartition.SubmitTx(transferFC))

	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPartition, testpartition.ANY_VALIDATOR, transferFC)
	require.NoError(t, err, "transfer fee credit tx failed")
	// send addFC
	addFC := NewAddFC(t, network.RootPartition.Nodes[0].RootSigner,
		NewAddFCAttr(t, network.RootPartition.Nodes[0].RootSigner,
			WithTransferFCTx(transferFCRecord),
			WithTransferFCProof(transferFCProof),
			WithFCOwnerCondition(templates.AlwaysTrueBytes()),
		),
		testtransaction.WithUnitId(fcrID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(transactions.PayloadTypeAddFeeCredit),
	)
	require.NoError(t, moneyPartition.SubmitTx(addFC))
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, addFC), test.WaitDuration, test.WaitTick)
	return transferFCRecord.TransactionOrder
}
