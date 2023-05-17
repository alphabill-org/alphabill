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
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var (
	FCRID     = uint256.NewInt(88)
	FCRAmount = uint64(1e8)
)

// CreateFeeCredit creates fee credit to be able to spend initial bill
func CreateFeeCredit(t *testing.T, initialBillID []byte, network *testpartition.AlphabillPartition) *types.TransactionOrder {
	// send transferFC
	fcrIDBytes := FCRID.Bytes32()
	transferFC := testfc.NewTransferFC(t,
		testfc.NewTransferFCAttr(
			testfc.WithBacklink(nil),
			testfc.WithAmount(FCRAmount),
			testfc.WithTargetRecordID(fcrIDBytes[:]),
		),
		testtransaction.WithUnitId(initialBillID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(transactions.PayloadTypeTransferFeeCredit),
	)
	err := network.SubmitTx(transferFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferFC, network), test.WaitDuration, test.WaitTick)

	// send addFC
	_, transferFCProof, transactionRecord, err := network.GetBlockProof(transferFC)
	require.NoError(t, err)
	addFC := testfc.NewAddFC(t, network.RootSigners[0],
		testfc.NewAddFCAttr(t, network.RootSigners[0],
			testfc.WithTransferFCTx(transactionRecord),
			testfc.WithTransferFCProof(transferFCProof),
			testfc.WithFCOwnerCondition(script.PredicateAlwaysTrue()),
		),
		testtransaction.WithPayloadType(transactions.PayloadTypeAddFeeCredit),
		testtransaction.WithUnitId(fcrIDBytes[:]),
	)
	err = network.SubmitTx(addFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(addFC, network), test.WaitDuration, test.WaitTick)
	return transferFC
}
