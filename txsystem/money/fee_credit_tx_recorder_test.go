package money

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/testutils/transaction"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

var (
	moneySystemIDString   = string(moneySystemID)
	systemIDUnknown       = []byte{1, 2, 3, 4}
	unknownSystemIDString = string(systemIDUnknown)
)

func TestTxRecording(t *testing.T) {
	f := newFeeCreditTxRecorder(nil, nil, nil)
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	transferFCAmount := uint64(10)
	transferFCFee := uint64(1)
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

	closeFCAmount := uint64(20)
	closeFCFee := uint64(2)
	reclaimFCFee := uint64(3)

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

	addedCredit := f.getAddedCredit(moneySystemIDString)
	require.EqualValues(t, transferFCAmount-transferFCFee, addedCredit)
	require.EqualValues(t, 0, f.getAddedCredit(unknownSystemIDString))

	reclaimedCredit := f.getReclaimedCredit(moneySystemIDString)
	require.EqualValues(t, closeFCAmount-closeFCFee, reclaimedCredit)
	require.EqualValues(t, 0, f.getReclaimedCredit(unknownSystemIDString))

	spentFees := f.getSpentFeeSum()
	require.EqualValues(t, transferFCFee+reclaimFCFee, spentFees)
}
