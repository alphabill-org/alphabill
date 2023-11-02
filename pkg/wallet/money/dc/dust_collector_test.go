package dc

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/testutil"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

func TestDC_OK(t *testing.T) {
	// create wallet with 3 normal bills
	bills := []*wallet.Bill{createBill(1), createBill(2), createBill(3)}
	targetBill := bills[2]
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then swap contains two dc txs
	attr := &money.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(&attr)
	require.NoError(t, err)
	require.EqualValues(t, 3, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestDCWontRunForSingleBill(t *testing.T) {
	// create backend with single bill
	bills := []*wallet.Bill{createBill(1)}
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)

	// then swap proof is not returned
	require.Nil(t, dcResult)

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestAllBillsAreSwapped_WhenWalletBillCountEqualToMaxBillCount(t *testing.T) {
	// create backend with bills = max dust collection bill count
	maxBillsPerDC := 10
	bills := make([]*wallet.Bill, maxBillsPerDC)
	for i := 0; i < maxBillsPerDC; i++ {
		bills[i] = createBill(uint64(i))
	}
	targetBill := bills[maxBillsPerDC-1]
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, maxBillsPerDC, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)

	// then swap tx should be returned
	require.NotNil(t, dcResult.SwapProof)
	require.EqualValues(t, targetBill.GetID(), dcResult.SwapProof.TxRecord.TransactionOrder.UnitID())

	// and swap contains correct dc transfers
	swapAttr := &money.SwapDCAttributes{}
	swapTxo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = swapTxo.UnmarshalAttributes(swapAttr)
	require.NoError(t, err)
	require.Len(t, swapAttr.DcTransfers, maxBillsPerDC-1)
	require.Len(t, swapAttr.DcTransferProofs, maxBillsPerDC-1)
	require.EqualValues(t, 36, swapAttr.TargetValue)
	require.EqualValues(t, targetBill.GetID(), swapTxo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestOnlyFirstNBillsAreSwapped_WhenBillCountOverLimit(t *testing.T) {
	// create backend with bills = max dust collection bill count
	maxBillsPerDC := 10
	billCountInWallet := 15
	bills := make([]*wallet.Bill, billCountInWallet)
	for i := 0; i < billCountInWallet; i++ {
		bills[i] = createBill(uint64(i))
	}
	targetBill := bills[billCountInWallet-1]
	backendMockWrapper := newBackendAPIMock(t, bills)

	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, maxBillsPerDC, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then swap contains correct dc transfers
	swapTxo := dcResult.SwapProof.TxRecord.TransactionOrder
	swapAttr := &money.SwapDCAttributes{}
	err = swapTxo.UnmarshalAttributes(swapAttr)
	require.EqualValues(t, targetBill.GetID(), swapTxo.UnitID())
	require.NoError(t, err)
	require.Len(t, swapAttr.DcTransfers, maxBillsPerDC)
	require.Len(t, swapAttr.DcTransferProofs, maxBillsPerDC)
	require.EqualValues(t, 45, swapAttr.TargetValue)
	require.EqualValues(t, targetBill.GetID(), swapTxo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_UnconfirmedDCTxs_NewSwapIsSent(t *testing.T) {
	// create wallet with 3 normal bills with one of them locked
	ctx := context.Background()
	bills := []*wallet.Bill{
		createBill(1),
		createBill(2),
		createBill(3),
	}
	targetBill := bills[2]
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// when locked unit exists in wallet
	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		targetBill.GetTxHash(),
		money.DefaultSystemIdentifier,
		unitlock.LockReasonCollectDust,
	))
	require.NoError(t, err)

	// and dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then new swap should be sent
	attr := &money.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.EqualValues(t, 3, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_TargetUnitSwapIsConfirmed_ProofIsReturned(t *testing.T) {
	// create wallet with locked unit that has confirmed swap tx
	ctx := context.Background()
	bills := []*wallet.Bill{createBill(10)}
	targetBill := bills[0]
	proofs := []*wallet.Proof{createProofWithSwapTx(t, targetBill)}
	backendMockWrapper := newBackendAPIMock(t, bills, withProofs(proofs))
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		targetBill.GetTxHash(),
		money.DefaultSystemIdentifier,
		unitlock.LockReasonCollectDust,
		unitlock.NewTransaction(proofs[0].TxRecord.TransactionOrder),
	))
	require.NoError(t, err)

	// when dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)

	// then confirmed swap proof is returned
	require.NotNil(t, dcResult.SwapProof)
	require.Equal(t, proofs[0], dcResult.SwapProof)

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_TargetUnitIsInvalid_NewSwapIsSent(t *testing.T) {
	// create wallet with 2 dc bills and 2 normal bills, and a target bill
	ctx := context.Background()
	targetBill := createBill(5)
	bills := []*wallet.Bill{
		createDCBill(1, targetBill),
		createDCBill(2, targetBill),
		createBill(3),
		createBill(4),
		targetBill,
	}
	proofs := []*wallet.Proof{
		createProofWithDCTx(t, bills[0], targetBill, 10),
		createProofWithDCTx(t, bills[1], targetBill, 10),
	}
	backendMockWrapper := newBackendAPIMock(t, bills, withProofs(proofs))
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// lock target bill, but change tx hash so that locked unit is considered invalid
	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		hash.Sum256(targetBill.GetTxHash()),
		money.DefaultSystemIdentifier,
		unitlock.LockReasonCollectDust,
	))
	require.NoError(t, err)

	// when dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then new swap should be sent using only the normal bill
	attr := &money.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.EqualValues(t, 7, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_DCOnSecondAccountDoesNotClearFirstAccountUnitLock(t *testing.T) {
	// create wallet with 3 bills
	var bills []*wallet.Bill
	for i := 0; i < 3; i++ {
		bills = append(bills, createBill(uint64(i)))
	}
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()

	w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// lock random bill before swap
	lockedUnit := unitlock.NewLockedUnit([]byte{200}, []byte{1}, []byte{2}, money.DefaultSystemIdentifier, unitlock.LockReasonCollectDust)
	err := unitLocker.LockUnit(lockedUnit)
	require.NoError(t, err)

	// when dc is run for account 1
	swapTx, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, swapTx)

	// then the previously locked unit is not changed
	actualLockedUnit, err := unitLocker.GetUnit(lockedUnit.AccountID, lockedUnit.UnitID)
	require.NoError(t, err)
	require.Equal(t, lockedUnit, actualLockedUnit)

	// and no locked unit exists for account 1
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_ServerAndClientSideLock(t *testing.T) {
	t.Run("bill is locked client side but not server side", func(t *testing.T) {
		// create wallet with 2 dc bills, 1 normal bill and a locally locked target bill
		// test that client side bill is unlocked and swap is retried
		ctx := context.Background()
		targetBill := createBill(4)
		bills := []*wallet.Bill{
			createBill(1),
			targetBill,
		}
		backendMockWrapper := newBackendAPIMock(t, bills)
		unitLocker := unitlock.NewInMemoryUnitLocker()
		w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

		// when locked unit exists in wallet
		err := unitLocker.LockUnit(unitlock.NewLockedUnit(
			backendMockWrapper.accountKey.PubKey,
			targetBill.GetID(),
			targetBill.GetTxHash(),
			money.DefaultSystemIdentifier,
			unitlock.LockReasonCollectDust,
		))
		require.NoError(t, err)

		// and dc is run
		dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
		require.NoError(t, err)
		require.NotNil(t, dcResult.SwapProof)

		// the normal bill should be swapped into the locked bill
		attr := &money.SwapDCAttributes{}
		txo := dcResult.SwapProof.TxRecord.TransactionOrder
		err = txo.UnmarshalAttributes(attr)
		require.NoError(t, err)
		require.EqualValues(t, 1, attr.TargetValue)
		require.Len(t, attr.DcTransfers, 1)
		require.Len(t, attr.DcTransferProofs, 1)
		require.EqualValues(t, targetBill.GetID(), txo.UnitID())

		// and no locked units exists
		units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
		require.NoError(t, err)
		require.Len(t, units, 0)
	})
	t.Run("bill is locked server side but not client side", func(t *testing.T) {
		// create wallet with 2 normal bills and 1 server side locked target bill
		// test that server side locked bill is ignored i.e. general case of
		// "Dust collection interrupted mid-process, client device lost or destroyed"
		ctx := context.Background()
		targetBill := createBill(2)
		bills := []*wallet.Bill{
			createBill(1),
			targetBill,
			{
				Id:     util.Uint64ToBytes32(3),
				Value:  3,
				TxHash: hash.Sum256([]byte{byte(3)}),
				Locked: unitlock.LockReasonCollectDust,
			},
		}
		backendMockWrapper := newBackendAPIMock(t, bills)
		unitLocker := unitlock.NewInMemoryUnitLocker()
		w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

		// when dc is run
		dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
		require.NoError(t, err)
		require.NotNil(t, dcResult.SwapProof)

		// then server side locked bill is ignored i.e. general case of
		// Dust collection interrupted mid-process, client device lost or destroyed, will be handled later
		attr := &money.SwapDCAttributes{}
		txo := dcResult.SwapProof.TxRecord.TransactionOrder
		err = txo.UnmarshalAttributes(attr)
		require.NoError(t, err)
		require.EqualValues(t, 1, attr.TargetValue)
		require.Len(t, attr.DcTransfers, 1)
		require.Len(t, attr.DcTransferProofs, 1)
		require.EqualValues(t, targetBill.GetID(), txo.UnitID())

		// and no locked units exists
		units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
		require.NoError(t, err)
		require.Len(t, units, 0)
	})
	t.Run("bill is locked server side and client side", func(t *testing.T) {
		// create wallet with 1 normal bill and 1 server and client side locked target bill
		// test that swap is sent using the existing target bill
		ctx := context.Background()
		targetBill := &wallet.Bill{
			Id:     util.Uint64ToBytes32(1),
			Value:  1,
			TxHash: hash.Sum256([]byte{byte(1)}),
			Locked: unitlock.LockReasonCollectDust,
		}
		bills := []*wallet.Bill{
			targetBill,
			createBill(2),
		}
		proofs := []*wallet.Proof{
			createProofWithLockTx(t, targetBill, 10),
		}
		backendMockWrapper := newBackendAPIMock(t, bills, withProofs(proofs))
		unitLocker := unitlock.NewInMemoryUnitLocker()
		err := unitLocker.LockUnit(unitlock.NewLockedUnit(
			backendMockWrapper.accountKey.PubKey,
			targetBill.GetID(),
			targetBill.GetTxHash(),
			money.DefaultSystemIdentifier,
			unitlock.LockReasonCollectDust,
			unitlock.NewTransaction(proofs[0].TxRecord.TransactionOrder),
		))
		require.NoError(t, err)
		w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

		// when dc is run
		dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
		require.NoError(t, err)
		require.NotNil(t, dcResult.SwapProof)

		// then swap is done using the existing locked bill
		attr := &money.SwapDCAttributes{}
		txo := dcResult.SwapProof.TxRecord.TransactionOrder
		err = txo.UnmarshalAttributes(attr)
		require.NoError(t, err)
		require.EqualValues(t, 2, attr.TargetValue)
		require.Len(t, attr.DcTransfers, 1)
		require.Len(t, attr.DcTransferProofs, 1)
		require.EqualValues(t, targetBill.GetID(), txo.UnitID())

		// and bill is unlocked
		units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
		require.NoError(t, err)
		require.Len(t, units, 0)
	})
}

func TestExistingDC_FailedSwapTx(t *testing.T) {
	// create wallet with 3 dc bills and a target bill,
	// and locally locked unit with unconfirmed swap tx
	// new swap should be sent using the 3 dc transfers
	ctx := context.Background()
	targetBill := &wallet.Bill{
		Id:     util.Uint64ToBytes32(5),
		Value:  5,
		TxHash: hash.Sum256([]byte{byte(5)}),
		Locked: unitlock.LockReasonCollectDust,
	}
	bills := []*wallet.Bill{
		createDCBill(1, targetBill),
		createDCBill(2, targetBill),
		createDCBill(3, targetBill),
		targetBill,
	}
	proofs := []*wallet.Proof{
		createProofWithDCTx(t, bills[0], targetBill, 10),
		createProofWithDCTx(t, bills[1], targetBill, 10),
		createProofWithDCTx(t, bills[2], targetBill, 10),
	}
	backendMockWrapper := newBackendAPIMock(t, bills, withProofs(proofs))
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// lock target bill with swap tx
	swapTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(targetBill.Id),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithAttributes(money.SwapDCAttributes{}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{Timeout: 0}),
	)
	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		targetBill.GetTxHash(),
		money.DefaultSystemIdentifier,
		unitlock.LockReasonCollectDust,
		unitlock.NewTransaction(swapTx),
	))
	require.NoError(t, err)

	// when dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then new swap should be sent using dc bills from server
	attr := &money.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.EqualValues(t, 6, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 3)
	require.Len(t, attr.DcTransferProofs, 3)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and unit is unlocked
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_FailedDCTx(t *testing.T) {
	// create wallet with 3 dc bills and a target bill,
	// and locally locked unit with 2-of-3 of the unconfirmed dc txs
	// new swap should be sent using the 3 dc transfers
	ctx := context.Background()
	targetBill := &wallet.Bill{
		Id:     util.Uint64ToBytes32(5),
		Value:  5,
		TxHash: hash.Sum256([]byte{byte(5)}),
		Locked: unitlock.LockReasonCollectDust,
	}
	bills := []*wallet.Bill{
		createDCBill(1, targetBill),
		createDCBill(2, targetBill),
		createDCBill(3, targetBill),
		targetBill,
	}
	proofs := []*wallet.Proof{
		createProofWithDCTx(t, bills[0], targetBill, 10),
		createProofWithDCTx(t, bills[1], targetBill, 10),
		createProofWithDCTx(t, bills[2], targetBill, 10),
	}
	backendMockWrapper := newBackendAPIMock(t, bills, withProofs(proofs))
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// lock target bill with 2-of-3 of the dust transfers
	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		targetBill.GetTxHash(),
		money.DefaultSystemIdentifier,
		unitlock.LockReasonCollectDust,
		unitlock.NewTransaction(proofs[0].TxRecord.TransactionOrder),
		unitlock.NewTransaction(proofs[1].TxRecord.TransactionOrder),
	))
	require.NoError(t, err)

	// when dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then new swap should be sent using only 2 of the 3 dc txs
	attr := &money.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.EqualValues(t, 3, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and unit is unlocked
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_FailedLockTx(t *testing.T) {
	// create wallet with 2 normal bills and a target bill,
	// and locally locked unit with unconfirmed lock tx
	// test that local bill is unlocked and new swap is sent
	ctx := context.Background()
	targetBill := &wallet.Bill{
		Id:     util.Uint64ToBytes32(5),
		Value:  5,
		TxHash: hash.Sum256([]byte{byte(5)}),
		Locked: unitlock.LockReasonCollectDust,
	}
	bills := []*wallet.Bill{
		createBill(1),
		createBill(2),
		targetBill,
	}
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(money.DefaultSystemIdentifier, 10, 10, backendMockWrapper.backendMock, unitLocker, logger.New(t))

	// lock target bill with the lock tx
	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		targetBill.GetTxHash(),
		money.DefaultSystemIdentifier,
		unitlock.LockReasonCollectDust,
		unitlock.NewTransaction(testtransaction.NewTransactionOrder(t,
			testtransaction.WithPayloadType(money.PayloadTypeLock),
			testtransaction.WithClientMetadata(&types.ClientMetadata{Timeout: 0})),
		),
	))
	require.NoError(t, err)

	// when dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then new swap should be sent
	attr := &money.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.EqualValues(t, 3, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and unit is unlocked
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func createBill(value uint64) *wallet.Bill {
	return &wallet.Bill{
		Id:     util.Uint64ToBytes32(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
}

func createDCBill(value uint64, targetBill *wallet.Bill) *wallet.Bill {
	srcBill := &wallet.Bill{
		Id:     util.Uint64ToBytes32(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
	srcBill.DCTargetUnitID = targetBill.GetID()
	srcBill.DCTargetUnitBacklink = targetBill.TxHash
	return srcBill
}

type (
	dustCollectionBackendMock struct {
		backendMock *testutil.BackendAPIMock
		recordedTxs map[string]*types.TransactionOrder
		accountKey  *account.AccountKey
		signer      crypto.Signer
		verifier    crypto.Verifier
	}
	Options struct {
		proofs []*wallet.Proof
	}
	Option func(*Options)
)

func withProofs(proofs []*wallet.Proof) Option {
	return func(c *Options) {
		c.proofs = proofs
	}
}

func newBackendAPIMock(t *testing.T, bills []*wallet.Bill, opts ...Option) *dustCollectionBackendMock {
	recordedTxs := make(map[string]*types.TransactionOrder)
	signer, _ := testsig.CreateSignerAndVerifier(t)
	accountKeys, _ := account.NewKeys("")
	accountKey := accountKeys.AccountKey
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcb := &wallet.Bill{Id: money.NewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256), Value: 100 * 1e8}

	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	backendMock := &testutil.BackendAPIMock{
		GetRoundNumberFn: func() (uint64, error) {
			return 0, nil
		},
		ListBillsFn: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return &backend.ListBillsResponse{Bills: bills}, nil
		},
		GetBillsFn: func(pubKey []byte) ([]*wallet.Bill, error) {
			var nonDCBills []*wallet.Bill
			for _, b := range bills {
				if len(b.DCTargetUnitID) == 0 {
					nonDCBills = append(nonDCBills, b)
				}
			}
			return nonDCBills, nil
		},
		GetFeeCreditBillFn: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return fcb, nil
		},
		PostTransactionsFn: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		GetTxProofFn: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if found {
				txRecord := &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: txbuilder.MaxFee}}
				txProof := testblock.CreateProof(t, txRecord, signer)
				return &wallet.Proof{TxRecord: txRecord, TxProof: txProof}, nil
			}
			for _, proof := range options.proofs {
				if bytes.Equal(proof.TxRecord.TransactionOrder.UnitID(), unitID) {
					return proof, nil
				}
			}
			return nil, nil
		},
	}
	return &dustCollectionBackendMock{
		backendMock: backendMock,
		recordedTxs: recordedTxs,
		signer:      signer,
		verifier:    verifier,
		accountKey:  accountKey,
	}
}

func createProofWithDCTx(t *testing.T, b *wallet.Bill, targetBill *wallet.Bill, timeout uint64) *wallet.Proof {
	keys, _ := account.NewKeys("")
	dcTx, err := txbuilder.NewDustTx(keys.AccountKey, []byte{0, 0, 0, 0}, b, targetBill.Id, targetBill.TxHash, timeout)
	require.NoError(t, err)
	return createProofForTx(dcTx)
}

func createProofWithSwapTx(t *testing.T, b *wallet.Bill) *wallet.Proof {
	txo := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(b.GetID()),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithAttributes(money.SwapDCAttributes{}),
	)
	return createProofForTx(txo)
}

func createProofWithLockTx(t *testing.T, b *wallet.Bill, timeout uint64) *wallet.Proof {
	keys, _ := account.NewKeys("")
	tx, err := txbuilder.NewLockTx(keys.AccountKey, []byte{0, 0, 0, 0}, b, unitlock.LockReasonCollectDust, timeout)
	require.NoError(t, err)
	return createProofForTx(tx)
}

func createProofForTx(tx *types.TransactionOrder) *wallet.Proof {
	txRecord := &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: txbuilder.MaxFee}}
	txProof := &wallet.Proof{
		TxRecord: txRecord,
		TxProof: &types.TxProof{
			BlockHeaderHash:    []byte{0},
			Chain:              []*types.GenericChainItem{{Hash: []byte{0}}},
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 10}},
		},
	}
	return txProof
}
