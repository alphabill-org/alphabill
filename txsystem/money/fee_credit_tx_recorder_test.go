package money

import (
	"testing"

	fct "github.com/alphabill-org/alphabill/txsystem/fc/types"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
)

func TestTxRecording(t *testing.T) {
	const systemIDUnknown types.SystemID = 0x01020304
	f := newFeeCreditTxRecorder(nil, 0, nil)
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	transferFCAmount := fct.Fee(10)
	transferFCFee := fct.Fee(1)
	attr := testutils.NewTransferFCAttr(testutils.WithAmount(transferFCAmount))
	f.recordTransferFC(
		&transferFeeCreditTx{
			tx: testutils.NewTransferFC(t,
				attr,
				testtransaction.WithSystemID(moneySystemID),
			),
			fee:  transferFCFee,
			attr: attr,
		},
	)

	closeFCAmount := fct.Fee(20)
	closeFCFee := fct.Fee(2)
	reclaimFCFee := fct.Fee(3)

	closeFCAttr := testutils.NewCloseFCAttr(testutils.WithCloseFCAmount(closeFCAmount))
	closureTx := testutils.WithReclaimFCClosureTx(
		&types.TransactionRecord{
			TransactionOrder: testutils.NewCloseFC(t, closeFCAttr),
			ServerMetadata:   &types.ServerMetadata{ActualFee: closeFCFee},
		},
	)
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
