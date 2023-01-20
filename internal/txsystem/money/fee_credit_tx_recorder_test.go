package money

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/stretchr/testify/require"
)

var (
	moneySystemID         = []byte{0, 0, 0, 0}
	moneySystemIDString   = string(moneySystemID)
	systemIDUnknown       = []byte{1, 2, 3, 4}
	unknownSystemIDString = string(systemIDUnknown)
)

func TestTxRecording(t *testing.T) {
	f := newFeeCreditTxRecorder()
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	transferFCAmount := uint64(10)
	transferFCFee := uint64(1)
	f.recordTransferFC(
		testfc.NewTransferFC(t,
			testfc.NewTransferFCAttr(testfc.WithAmount(transferFCAmount)),
			testtransaction.WithSystemID(moneySystemID),
			testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: transferFCFee}),
		),
	)

	closeFCAmount := uint64(20)
	closeFCFee := uint64(2)
	reclaimFCFee := uint64(3)
	f.recordReclaimFC(testfc.NewReclaimFC(t, signer,
		testfc.NewReclFCAttr(t, signer, testfc.WithReclFCCloseFCTx(
			testfc.NewCloseFC(t,
				testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(closeFCAmount)),
				testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: closeFCFee})).Transaction,
		)),
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: reclaimFCFee}),
	))

	addedCredit := f.getAddedCredit(moneySystemIDString)
	require.EqualValues(t, transferFCAmount, addedCredit)
	require.EqualValues(t, 0, f.getAddedCredit(unknownSystemIDString))

	reclaimedCredit := f.getReclaimedCredit(moneySystemIDString)
	require.EqualValues(t, closeFCAmount-closeFCFee, reclaimedCredit)
	require.EqualValues(t, 0, f.getReclaimedCredit(unknownSystemIDString))

	spentFees := f.getSpentFeeSum()
	require.EqualValues(t, transferFCFee+reclaimFCFee, spentFees)
}
