package fees

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

var (
	moneySystemID  = []byte{0, 0, 0, 0}
	tokensSystemID = []byte{0, 0, 0, 2}
)

/*
Wallet has single bill with value 1.00000002
Add fee credit with value 1.00000001
Result should have 0 balance, 1.00000000 in fee credit and no error and any larger amount should return error.
*/
func TestAddFeeCredit_OK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{{
		Id:     []byte{1},
		Value:  100000002,
		TxHash: []byte{2},
	}}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// verify that entire bill amount can be added
	proofs, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000001})
	require.NoError(t, err)
	require.NotNil(t, proofs)
	require.NotNil(t, proofs.TransferFC)
	require.NotNil(t, proofs.AddFC)
}

/*
Wallet has multiple bills
Add fee credit with amount higher than the biggest bill
Result should have 2 transFC txs with the combined amount that matches what was requested
*/
func TestAddFeeCredit_MultipleBills(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{
		{
			Id:     []byte{1},
			Value:  100000001,
			TxHash: []byte{2},
		},
		{
			Id:     []byte{2},
			Value:  100000002,
			TxHash: []byte{3},
		},
		{
			Id:     []byte{3},
			Value:  100000003,
			TxHash: []byte{4},
		}}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// verify that there are 2 pairs of txs sent and that the amounts match
	proofs, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 200000000})
	require.NoError(t, err)
	require.NotNil(t, proofs)
	require.Len(t, proofs.TransferFC, 2)
	// first transfer amount should match the biggest bill
	firstTransFCAttr := &transactions.TransferFeeCreditAttributes{}
	err = proofs.TransferFC[0].TxRecord.TransactionOrder.UnmarshalAttributes(firstTransFCAttr)
	require.NoError(t, err)
	require.EqualValues(t, []byte{3}, proofs.TransferFC[0].TxRecord.TransactionOrder.UnitID())
	require.Equal(t, uint64(100000003), firstTransFCAttr.Amount)
	// second transfer amount should match the remaining value
	secondTransFCAttr := &transactions.TransferFeeCreditAttributes{}
	err = proofs.TransferFC[1].TxRecord.TransactionOrder.UnmarshalAttributes(secondTransFCAttr)
	require.NoError(t, err)
	require.EqualValues(t, []byte{2}, proofs.TransferFC[1].TxRecord.TransactionOrder.UnitID())
	require.Equal(t, uint64(200000000-100000003), secondTransFCAttr.Amount)
	require.Len(t, proofs.AddFC, 2)
}

/*
Wallet has no bills
Trying to create fee credit should return error "wallet does not contain any bills"
*/
func TestAddFeeCredit_NoBillsReturnsError(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// verify that error is returned
	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.ErrorContains(t, err, "wallet does not contain any bills")
}

/*
Wallet contains locked bill for reclaim
Return error "wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit"
*/
func TestAddFeeCredit_WalletContainsLockedBillForReclaim(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// lock bill with LockReasonReclaimFees
	err = unitLocker.LockUnit(unitlock.NewLockedUnit(accountKey.PubKey, []byte{1}, []byte{200}, moneySystemID, unitlock.LockReasonReclaimFees))
	require.NoError(t, err)

	// verify error is returned
	_, err = feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.ErrorContains(t, err, "wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit")
}

/*
Wallet has two bills: one locked for dust collection and one normal not locked bill.
Adding fee credit should use the unlocked bill not change the locked bill.
*/
func TestAddFeeCredit_WalletContainsLockedBillForDustCollection(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{
		{
			Id:     []byte{1},
			Value:  100000001,
			TxHash: []byte{1},
		},
		{
			Id:     []byte{2},
			Value:  100000002,
			TxHash: []byte{2},
		},
	}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// lock the first bill with reason LockReasonCollectDust
	publicKey, err := am.GetPublicKey(0)
	require.NoError(t, err)
	lockedDCBill := unitlock.NewLockedUnit(publicKey, []byte{1}, []byte{1}, moneySystemID, unitlock.LockReasonCollectDust)
	require.NoError(t, unitLocker.LockUnit(lockedDCBill))

	// verify that the second bill can be added to fee credit
	proofs, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.NoError(t, err)
	require.NotNil(t, proofs)
	require.Len(t, proofs.TransferFC, 1)
	require.Len(t, proofs.AddFC, 1)
	require.EqualValues(t, []byte{2}, proofs.TransferFC[0].TxRecord.TransactionOrder.UnitID())

	// and the first bill remains locked
	lockedDCBillAfter, err := unitLocker.GetUnit(publicKey, []byte{1})
	require.NoError(t, err)
	require.Equal(t, lockedDCBill, lockedDCBillAfter)
}

/*
Wallet contains locked bill for TransferFC and the tx is either:
1. confirmed => send addFC using the confirmed transferFC
2. timed out => create new tx
3. still pending => re-send the tx
*/
func TestAddFeeCredit_LockedBillForTransferFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	unitLocker := createUnitLocker(t)

	transferFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	transferFCProof := &wallet.Proof{TxRecord: transferFCRecord, TxProof: &types.TxProof{}}
	lockedUnitTxHash := []byte{200}
	lockedTransferFCBill := unitlock.NewLockedUnit(
		accountKey.PubKey,
		transferFCRecord.TransactionOrder.UnitID(),
		lockedUnitTxHash,
		moneySystemID,
		unitlock.LockReasonAddFees,
		unitlock.NewTransaction(transferFCRecord.TransactionOrder),
	)

	t.Run("transferFC confirmed => send addFC using the confirmed transferFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedTransferFCBill)
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{string(lockedTransferFCBill.UnitID): transferFCProof},
		}

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)

		// then addFC tx must be sent using the confirmed transferFC
		require.NotNil(t, res)
		require.NotNil(t, res.TransferFC)
		require.NotNil(t, res.AddFC)

		sentAddFCAttr := &transactions.AddFeeCreditAttributes{}
		err = res.AddFC[0].TxRecord.TransactionOrder.UnmarshalAttributes(sentAddFCAttr)
		require.NoError(t, err)
		require.Equal(t, transferFCRecord, sentAddFCAttr.FeeCreditTransfer)

		// and bill must be unlocked
		units, err := unitLocker.GetUnits(accountKey.PubKey)
		require.NoError(t, err)
		require.Len(t, units, 0)
	})

	t.Run("transferFC timed out => create new transferFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedTransferFCBill)
		require.NoError(t, err)

		// mock tx timed out and add bill to wallet
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedTransferFCBill.Transactions[0].TxOrder.Timeout(),
			bills: []*wallet.Bill{{
				Id:     []byte{123},
				Value:  100,
				TxHash: []byte{2},
			}},
		}

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.TransferFC)
		require.NotNil(t, res.AddFC)

		// then new transferFC must be sent
		require.EqualValues(t, []byte{123}, res.TransferFC[0].TxRecord.TransactionOrder.UnitID())

		// and bill must be unlocked
		units, err := unitLocker.GetUnits(accountKey.PubKey)
		require.NoError(t, err)
		require.Len(t, units, 0)
	})

	t.Run("transferFC still pending => re-send the transferFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedTransferFCBill)
		require.NoError(t, err)

		// mock tx not yet timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedTransferFCBill.Transactions[0].TxOrder.Timeout() - 1,
		}

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)

		// then the pending transferFC must be re-sent
		require.Equal(t, transferFCProof, res.TransferFC[0])

		// and bill must be unlocked
		units, err := unitLocker.GetUnits(accountKey.PubKey)
		require.NoError(t, err)
		require.Len(t, units, 0)
	})
}

/*
Wallet contains locked bill for AddFC and tx is either:
1. confirmed => return no error (and optionally the fee txs)
2. still pending => re-send the tx
3. partially timed out => create new tx
4. fully timed out => return money lost error
*/
func TestAddFeeCredit_LockedBillForAddFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	addFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewAddFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{Timeout: 5, MaxTransactionFee: 2})),
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}
	addFCProof := &wallet.Proof{TxRecord: addFCRecord, TxProof: &types.TxProof{}}
	lockedUnitTxHash := []byte{200}
	lockedAddFCBill := unitlock.NewLockedUnit(
		accountKey.PubKey,
		addFCRecord.TransactionOrder.UnitID(),
		lockedUnitTxHash,
		moneySystemID,
		unitlock.LockReasonAddFees,
		unitlock.NewTransaction(addFCRecord.TransactionOrder),
	)

	t.Run("addFC confirmed => return no error (and optionally the fee txs)", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedAddFCBill)
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{string(lockedAddFCBill.UnitID): addFCProof},
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)

		// then no error must be returned (and optionally the fee txs can be returned)
		require.NotNil(t, res)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedAddFCBill.UnitID, accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("addFC still pending => re-send the addFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedAddFCBill)
		require.NoError(t, err)

		// mock tx timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedAddFCBill.Transactions[0].TxOrder.Timeout() - 1,
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res.AddFC)

		// then AddFC must be re-sent
		require.Equal(t, addFCRecord, res.AddFC[0].TxRecord)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedAddFCBill.UnitID, accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("addFC partially timed out => create new addFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedAddFCBill)
		require.NoError(t, err)

		// mock tx timed out
		// tx timeout (5) < round number (6) < latest addition time (10)
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedAddFCBill.Transactions[0].TxOrder.Timeout() + 1,
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.AddFC)

		// then new addFC must be sent using the existing transferFC
		// new addFC has new tx timeout = round number + txTimeoutBlockCount
		require.EqualValues(t, moneyBackendClient.roundNumber+txTimeoutBlockCount, res.AddFC[0].TxRecord.TransactionOrder.Timeout())

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedAddFCBill.UnitID, accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("addFC fully timed out => return money lost error", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedAddFCBill)
		require.NoError(t, err)

		// mock tx timed out
		// round number > latest addition time
		*moneyBackendClient = mockMoneyClient{
			roundNumber: 11,
		}

		// when fees are added
		// then money lost error must be returned
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.ErrorContains(t, err, "transferFC latestAdditionTime exceeded, locked fee credit is no longer usable")
		require.Nil(t, res)
	})
}

func TestAddFeeCreditForMoneyPartition_LockedBillExistsForTokensPartition(t *testing.T) {
	// create fee manager for money partition
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{
		fcb: &wallet.Bill{Value: 100, Id: []byte{111}},
		bills: []*wallet.Bill{
			{
				Id:     []byte{1}, // locked for tokens fee credit
				Value:  100000002,
				TxHash: []byte{2},
			},
			{
				Id:     []byte{2}, // not locked
				Value:  100000002,
				TxHash: []byte{3},
			},
		},
	}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// lock unit for tokens partition
	lockedUnit := unitlock.NewLockedUnit(
		accountKey.PubKey,
		[]byte{1},
		[]byte{100},
		tokensSystemID,
		unitlock.LockReasonAddFees,
	)
	require.NoError(t, unitLocker.LockUnit(lockedUnit))

	// when attempting to add fees for money partition
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})

	// then error must be returned
	require.ErrorIs(t, err, ErrLockedBillWrongPartition)
	require.Nil(t, res)

	// and bill must remain locked
	lockedUnitAfter, err := unitLocker.GetUnit(lockedUnit.AccountID, lockedUnit.UnitID)
	require.NoError(t, err)
	require.NotNil(t, lockedUnitAfter)
}

/*
Wallet contains locked bill for closeFC and the tx is either:
1. confirmed => send reclaimFC using the confirmed closeFC
2. timed out => create new tx
3. still pending => re-send the tx
*/
func TestReclaimFeeCredit_LockedBillForCloseFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	closeFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewCloseFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	closeFCProof := &wallet.Proof{TxRecord: closeFCRecord, TxProof: &types.TxProof{}}
	lockedUnitTxHash := []byte{200}
	lockedCloseFCBill := unitlock.NewLockedUnit(
		accountKey.PubKey,
		closeFCRecord.TransactionOrder.UnitID(),
		lockedUnitTxHash,
		moneySystemID,
		unitlock.LockReasonReclaimFees,
		unitlock.NewTransaction(closeFCRecord.TransactionOrder),
	)

	t.Run("closeFC confirmed => send reclaimFC using the confirmed closeFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedCloseFCBill)
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{string(lockedCloseFCBill.UnitID): closeFCProof},
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)

		// then reclaimFC tx must be sent using the confirmed closeFC
		require.NotNil(t, res)
		require.NotNil(t, res.CloseFC)
		require.NotNil(t, res.ReclaimFC)

		sentReclaimFCAttr := &transactions.ReclaimFeeCreditAttributes{}
		err = res.ReclaimFC.TxRecord.TransactionOrder.UnmarshalAttributes(sentReclaimFCAttr)
		require.NoError(t, err)
		require.Equal(t, closeFCRecord, sentReclaimFCAttr.CloseFeeCreditTransfer)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(accountKey.PubKey, lockedCloseFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("closeFC timed out => create new closeFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedCloseFCBill)
		require.NoError(t, err)

		// mock tx timed out and add bill to wallet
		*moneyBackendClient = mockMoneyClient{
			fcb:         &wallet.Bill{Value: 1e8, Id: []byte{111}},
			roundNumber: lockedCloseFCBill.Transactions[0].TxOrder.Timeout(),
			bills: []*wallet.Bill{{
				Id:     []byte{123},
				Value:  100,
				TxHash: []byte{2},
			}},
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.CloseFC)
		require.NotNil(t, res.ReclaimFC)

		// then new closeFC must be sent
		var actualCloseFCAttr *transactions.CloseFeeCreditAttributes
		err = res.CloseFC.TxRecord.TransactionOrder.UnmarshalAttributes(&actualCloseFCAttr)
		require.NoError(t, err)

		require.EqualValues(t, []byte{111}, res.CloseFC.TxRecord.TransactionOrder.UnitID()) // unitID matches FCB ID
		require.Equal(t, []byte{123}, actualCloseFCAttr.TargetUnitID)                       // target unitID matches target bill ID
		require.Equal(t, []byte{2}, actualCloseFCAttr.TargetUnitBacklink)                   // target unit backlink matches target bill txhash

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(accountKey.PubKey, lockedCloseFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("closeFC still pending => re-send the closeFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedCloseFCBill)
		require.NoError(t, err)

		// mock tx not yet timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedCloseFCBill.Transactions[0].TxOrder.Timeout() - 1,
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)

		// then the pending transferFC must be re-sent
		require.Equal(t, closeFCProof, res.CloseFC)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(accountKey.PubKey, lockedCloseFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})
}

/*
Wallet contains locked bill for ReclaimFC and tx is either:
1. confirmed => no error
2. still pending => re-send the tx
3. partially timed out => create new tx (target bill still usable)
4. fully timed out => return money lost error (target bill has been used)
*/
func TestReclaimFeeCredit_LockedBillForReclaimFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	reclaimFCOrder := testutils.NewReclaimFC(t, signer, nil)
	reclaimFCRecord := &types.TransactionRecord{
		TransactionOrder: reclaimFCOrder,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	reclaimFCProof := &wallet.Proof{TxRecord: reclaimFCRecord, TxProof: &types.TxProof{}}
	lockedUnitTxHash := []byte{200}
	lockedReclaimFCBill := unitlock.NewLockedUnit(
		accountKey.PubKey,
		reclaimFCRecord.TransactionOrder.UnitID(),
		lockedUnitTxHash,
		moneySystemID,
		unitlock.LockReasonReclaimFees,
		unitlock.NewTransaction(reclaimFCOrder),
	)

	t.Run("reclaimFC confirmed => return no error (and optionally the fee txs)", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedReclaimFCBill)
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{string(lockedReclaimFCBill.UnitID): reclaimFCProof},
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)

		// then reclaimFC proof must be returned
		require.NotNil(t, res)
		require.Equal(t, reclaimFCProof, res.ReclaimFC)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(accountKey.PubKey, reclaimFCOrder.UnitID())
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("reclaimFC still pending => re-send the reclaimFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedReclaimFCBill)
		require.NoError(t, err)

		// mock tx timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedReclaimFCBill.Transactions[0].TxOrder.Timeout() - 1,
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res.ReclaimFC)

		// then AddFC must be re-sent
		require.Equal(t, reclaimFCRecord, res.ReclaimFC.TxRecord)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(accountKey.PubKey, lockedReclaimFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("reclaimFC partially timed out => create new reclaimFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedReclaimFCBill)
		require.NoError(t, err)

		// mock tx timed out and return locked bill
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedReclaimFCBill.Transactions[0].TxOrder.Timeout() + 1,
			bills:       []*wallet.Bill{{Id: lockedReclaimFCBill.UnitID, TxHash: lockedUnitTxHash}},
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.ReclaimFC)

		// then new reclaimFC must be sent using the existing closeFC
		// new reclaimFC has new tx timeout = round number + tx timeout
		require.EqualValues(t, moneyBackendClient.roundNumber+txTimeoutBlockCount, res.ReclaimFC.TxRecord.TransactionOrder.Timeout())

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(accountKey.PubKey, lockedReclaimFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("reclaimFC fully timed out => return money lost error", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedReclaimFCBill)
		require.NoError(t, err)

		// mock tx timed out and no bills are available
		*moneyBackendClient = mockMoneyClient{
			roundNumber: 11,
		}

		// when fees are reclaimed
		// then money lost error must be returned
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.ErrorContains(t, err, "reclaimFC target unit is no longer usable")
		require.Nil(t, res)
	})
}

func TestReclaimFeeCreditForMoneyPartition_LockedBillExistsForTokensPartition(t *testing.T) {
	// create fee manager for money partition
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// lock unit for tokens partition
	lockedUnit := unitlock.NewLockedUnit(
		accountKey.PubKey,
		[]byte{1},
		[]byte{100},
		tokensSystemID,
		unitlock.LockReasonReclaimFees,
	)
	require.NoError(t, unitLocker.LockUnit(lockedUnit))

	// when attempting to reclaim fees for money partition
	res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})

	// then error must be returned
	require.ErrorIs(t, err, ErrLockedBillWrongPartition)
	require.Nil(t, res)

	// and bill must remain locked
	lockedUnitAfter, err := unitLocker.GetUnit(lockedUnit.AccountID, lockedUnit.UnitID)
	require.NoError(t, err)
	require.NotNil(t, lockedUnitAfter)
}

/*
Wallet has three bills: one locked for dust collection, one normal not locked bill and fee credit bill.
Reclaiming fee credit should target the unlocked bill not change the locked bill.
*/
func TestReclaimFeeCredit_WalletContainsLockedBillForDustCollection(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{
		fcb: &wallet.Bill{Value: 100, Id: []byte{111}},
		bills: []*wallet.Bill{
			{
				Id:     []byte{1},
				Value:  100000001,
				TxHash: []byte{1},
			},
			{
				Id:     []byte{2},
				Value:  100000002,
				TxHash: []byte{2},
			},
		}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// lock the first bill with reason LockReasonCollectDust
	publicKey, err := am.GetPublicKey(0)
	require.NoError(t, err)
	lockedDCBill := unitlock.NewLockedUnit(publicKey, []byte{1}, []byte{1}, moneySystemID, unitlock.LockReasonCollectDust)
	require.NoError(t, unitLocker.LockUnit(lockedDCBill))

	// verify that the second bill can be added to fee credit
	proofs, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.NoError(t, err)
	require.NotNil(t, proofs)
	require.NotNil(t, proofs.CloseFC)
	require.NotNil(t, proofs.ReclaimFC)

	var attr *transactions.CloseFeeCreditAttributes
	require.NoError(t, proofs.CloseFC.TxRecord.TransactionOrder.UnmarshalAttributes(&attr))
	require.Equal(t, []byte{2}, attr.TargetUnitID)

	// and the first bill remains locked
	lockedDCBillAfter, err := unitLocker.GetUnit(publicKey, []byte{1})
	require.NoError(t, err)
	require.Equal(t, lockedDCBill, lockedDCBillAfter)
}

func TestAddAndReclaimWithInsufficientCredit(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{
		fcb: &wallet.Bill{Value: 2, Id: []byte{111}},
		bills: []*wallet.Bill{{
			Id:     []byte{1},
			Value:  100000002,
			TxHash: []byte{2},
		}}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 2})
	require.ErrorIs(t, err, ErrMinimumFeeAmount)

	_, err = feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.ErrorIs(t, err, ErrMinimumFeeAmount)
}

func TestAddWithInsufficientBalance(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{
		fcb: &wallet.Bill{Value: 2, Id: []byte{111}},
		bills: []*wallet.Bill{{
			Id:     []byte{1},
			Value:  10,
			TxHash: []byte{2},
		}}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestAddWithSufficientBalanceInSmallBills(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{
		fcb: &wallet.Bill{Value: 2, Id: []byte{111}},
		bills: []*wallet.Bill{
			{
				Id:     []byte{1},
				Value:  1,
				TxHash: []byte{2},
			},
			{
				Id:     []byte{2},
				Value:  2,
				TxHash: []byte{3},
			},
			{
				Id:     []byte{3},
				Value:  1,
				TxHash: []byte{4},
			},
			{
				Id:     []byte{4},
				Value:  2,
				TxHash: []byte{5},
			},
		}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient, logger.New(t))

	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 4})
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func newMoneyPartitionFeeManager(am account.Manager, unitLocker UnitLocker, moneyTxPublisher TxPublisher, moneyBackendClient MoneyClient, log *slog.Logger) *FeeManager {
	return NewFeeManager(am, unitLocker, moneySystemID, moneyTxPublisher, moneyBackendClient, moneySystemID, moneyTxPublisher, moneyBackendClient, testFeeCreditRecordIDFromPublicKey, log)
}

func newAccountManager(t *testing.T) account.Manager {
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	t.Cleanup(am.Close)
	err = am.CreateKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	return am
}

func createUnitLocker(t *testing.T) *unitlock.UnitLocker {
	dbFile := filepath.Join(t.TempDir(), "unitstore")
	unitLocker, err := unitlock.NewUnitLocker(dbFile)
	require.NoError(t, err)
	return unitLocker
}

type mockMoneyClient struct {
	bills       []*wallet.Bill
	proofs      map[string]*wallet.Proof
	roundNumber uint64
	fcb         *wallet.Bill
}

func testFeeCreditRecordIDFromPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := hash.Sum256(pubKey)
	return money.NewFeeCreditRecordID(shardPart, unitPart)
}

func (m *mockMoneyClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	return m.roundNumber, nil
}

func (m *mockMoneyClient) GetBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error) {
	return m.bills, nil
}

func (m *mockMoneyClient) GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
	return m.fcb, nil
}

func (m *mockMoneyClient) GetLockedFeeCredit(ctx context.Context, unitID []byte, fcbID []byte) (*types.TransactionRecord, error) {
	return nil, nil
}

func (m *mockMoneyClient) GetClosedFeeCredit(ctx context.Context, fcbID []byte) (*types.TransactionRecord, error) {
	return nil, nil
}

func (m *mockMoneyClient) GetTxProof(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	return m.proofs[string(unitID)], nil
}

type mockMoneyTxPublisher struct {
	sendTxCount int
}

func (m *mockMoneyTxPublisher) SendTx(ctx context.Context, tx *types.TransactionOrder, _ []byte) (*wallet.Proof, error) {
	m.sendTxCount++
	return &wallet.Proof{
		TxRecord: &types.TransactionRecord{
			TransactionOrder: tx,
			ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
		},
		TxProof: &types.TxProof{},
	}, nil
}

func (m *mockMoneyTxPublisher) Close() {
}
