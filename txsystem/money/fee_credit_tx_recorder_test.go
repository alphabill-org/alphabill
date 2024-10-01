package money

import (
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

func TestTxRecording(t *testing.T) {
	const systemIDUnknown types.SystemID = 0x01020304
	f := newFeeCreditTxRecorder(nil, 0, nil)
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	transferFCAmount := uint64(10)
	transferFCFee := uint64(1)
	attr := testutils.NewTransferFCAttr(t, signer, testutils.WithAmount(transferFCAmount))
	f.recordTransferFC(
		&transferFeeCreditTx{
			tx: testutils.NewTransferFC(t, signer,
				attr,
				testtransaction.WithSystemID(moneySystemID),
			),
			fee:  transferFCFee,
			attr: attr,
		},
	)

	closeFCAmount := uint64(20)
	closeFCFee := uint64(2)
	reclaimFCFee := uint64(3)

	closeFCAttr := testutils.NewCloseFCAttr(testutils.WithCloseFCAmount(closeFCAmount))
	closureTx := testutils.WithReclaimFCClosureProof(&types.TxRecordProof{
		TxRecord: &types.TransactionRecord{
			TransactionOrder: testutils.NewCloseFC(t, signer, closeFCAttr),
			ServerMetadata:   &types.ServerMetadata{ActualFee: closeFCFee},
		},
	})
	newReclaimFCAttr := testutils.NewReclaimFCAttr(t, signer, closureTx)
	f.recordReclaimFC(
		&reclaimFeeCreditTx{
			tx:                  testutils.NewReclaimFC(t, signer, newReclaimFCAttr, testtransaction.WithSystemID(moneySystemID)),
			attr:                newReclaimFCAttr,
			closeFCTransferAttr: closeFCAttr,
			reclaimFee:          reclaimFCFee,
			closeFee:            closeFCFee,
		},
	)

	addedCredit := f.getAddedCredit(moneySystemID)
	require.EqualValues(t, transferFCAmount-transferFCFee, addedCredit)
	require.EqualValues(t, 0, f.getAddedCredit(systemIDUnknown))

	reclaimedCredit := f.getReclaimedCredit(moneySystemID)
	require.EqualValues(t, closeFCAmount-closeFCFee, reclaimedCredit)
	require.EqualValues(t, 0, f.getReclaimedCredit(systemIDUnknown))

	spentFees := f.getSpentFeeSum()
	require.EqualValues(t, transferFCFee+reclaimFCFee, spentFees)
}
