package testmoney

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var (
	FCRID     = util.Uint256ToBytes(uint256.NewInt(88))
	FCRAmount = uint64(1e8)
)

// CreateFeeCredit creates fee credit to be able to spend initial bill
func CreateFeeCredit(t *testing.T, initialBillID []byte, network *testpartition.AlphabillNetwork) *types.TransactionOrder {
	// send transferFC
	transferFC := testfc.NewTransferFC(t,
		testfc.NewTransferFCAttr(
			testfc.WithBacklink(nil),
			testfc.WithAmount(FCRAmount),
			testfc.WithTargetRecordID(FCRID),
		),
		testtransaction.WithUnitId(initialBillID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(transactions.PayloadTypeTransferFeeCredit),
	)
	moneyPartition, err := network.GetNodePartition([]byte{0, 0, 0, 0})
	require.NoError(t, err)
	err = moneyPartition.SubmitTx(transferFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, transferFC), 2*test.WaitDuration, 2*test.WaitTick)

	// send addFC
	_, transferFCProof, transferFCRecord, err := moneyPartition.GetTxProof(transferFC)
	require.NoError(t, err)
	addFC := testfc.NewAddFC(t, network.RootPartition.Nodes[0].RootSigner,
		testfc.NewAddFCAttr(t, network.RootPartition.Nodes[0].RootSigner,
			testfc.WithTransferFCTx(transferFCRecord),
			testfc.WithTransferFCProof(transferFCProof),
			testfc.WithFCOwnerCondition(script.PredicateAlwaysTrue()),
		),
		testtransaction.WithUnitId(FCRID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(transactions.PayloadTypeAddFeeCredit),
	)
	err = moneyPartition.SubmitTx(addFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, addFC), 2*test.WaitDuration, 2*test.WaitTick)
	return transferFCRecord.TransactionOrder
}
