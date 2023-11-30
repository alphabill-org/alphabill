package fees

import (
	"context"
	"log/slog"
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
Wallet has single bill with value 1.00000000
Add fee credit with the full value 1.00000000
TransferFCTx with 1.00000000 value and AddFCTx transactions should be sent.
*/
func TestAddFeeCredit_OK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{{
		Id:     []byte{1},
		Value:  100000000,
		TxHash: []byte{2},
	}}}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// add fees
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.NoError(t, err)
	require.Len(t, res.Proofs, 1)
	require.Nil(t, res.Proofs[0].LockFC)
	require.NotNil(t, res.Proofs[0].TransferFC)
	require.NotNil(t, res.Proofs[0].AddFC)

	// verify fee context is deleted
	pk, err := am.GetPublicKey(0)
	require.NoError(t, err)
	feeCtx, err := feeManagerDB.GetAddFeeContext(pk)
	require.NoError(t, err)
	require.Nil(t, feeCtx)

	// verify correct transferFC amount was sent
	var attr *transactions.TransferFeeCreditAttributes
	err = res.Proofs[0].TransferFC.TxRecord.TransactionOrder.UnmarshalAttributes(&attr)
	require.NoError(t, err)
	require.EqualValues(t, 100000000, attr.Amount)
}

/*
Wallet has single bill and fee credit bill,
when adding fees LockFCTx, TransferFCTx and AddFCTx transactions should be sent.
*/
func TestAddFeeCredit_ExistingFeeCreditBillOK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{
		bills: []*wallet.Bill{{
			Id:     []byte{1},
			Value:  100000000,
			TxHash: []byte{1},
		}},
		fcb: &wallet.Bill{
			Id:     []byte{2},
			Value:  100000000,
			TxHash: []byte{2},
		}}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// add fees
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Proofs, 1)
	proofs := res.Proofs[0]
	require.NotNil(t, proofs.LockFC)
	require.NotNil(t, proofs.TransferFC)
	require.NotNil(t, proofs.AddFC)

	// verify fee ctx is removed
	pk, err := am.GetPublicKey(0)
	require.NoError(t, err)
	feeCtx, err := feeManagerDB.GetAddFeeContext(pk)
	require.NoError(t, err)
	require.Nil(t, feeCtx)
}

/*
Wallet has multiple bills,
when adding fee credit with amount greater than the largest bill then
the result should have two sets of txs with the combined amount that matches what was requested
*/
func TestAddFeeCredit_MultipleBills(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{
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
			{
				Id:     []byte{3},
				Value:  100000003,
				TxHash: []byte{3},
			},
		},
		fcb: &wallet.Bill{
			Id:     []byte{4},
			Value:  100000004,
			TxHash: []byte{4},
		}}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// verify that there are 2 pairs of txs sent and that the amounts match
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 200000000})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Proofs, 2)
	proofs := res.Proofs

	// first transfer amount should match the largest bill
	firstTransFCAttr := &transactions.TransferFeeCreditAttributes{}
	err = proofs[0].TransferFC.TxRecord.TransactionOrder.UnmarshalAttributes(firstTransFCAttr)
	require.NoError(t, err)
	require.EqualValues(t, []byte{3}, proofs[0].TransferFC.TxRecord.TransactionOrder.UnitID())
	require.EqualValues(t, 100000003, firstTransFCAttr.Amount)

	// second transfer amount should match the remaining value
	secondTransFCAttr := &transactions.TransferFeeCreditAttributes{}
	err = proofs[1].TransferFC.TxRecord.TransactionOrder.UnmarshalAttributes(secondTransFCAttr)
	require.NoError(t, err)
	require.EqualValues(t, []byte{2}, proofs[1].TransferFC.TxRecord.TransactionOrder.UnitID())
	require.EqualValues(t, 200000000-100000003, secondTransFCAttr.Amount)
}

/*
Wallet has no bills.
Trying to add fee credit should return error "wallet does not contain any bills".
*/
func TestAddFeeCredit_NoBillsReturnsError(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{}}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// verify that error is returned
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.ErrorContains(t, err, "wallet does not contain any bills")
	require.Nil(t, res)
}

/*
Wallet contains existing context for reclaim. Trying to add fee credit should return error
"wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit"
*/
func TestAddFeeCredit_FeeManagerContainsExistingReclaimContext(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{}}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// create fee context for reclaim
	err = feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{})
	require.NoError(t, err)

	// verify error is returned
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.ErrorContains(t, err, "wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit")
	require.Nil(t, res)
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
			Locked: unitlock.LockReasonCollectDust,
		},
	}}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// verify that the smaller bill is used to create fee credit
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000001})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Proofs, 1)
	proofs := res.Proofs[0]
	require.Nil(t, proofs.LockFC)
	require.NotNil(t, proofs.TransferFC)
	require.NotNil(t, proofs.AddFC)
	require.EqualValues(t, []byte{1}, proofs.TransferFC.TxRecord.TransactionOrder.UnitID())
}

func TestAddFeeCreditForMoneyPartition_ExistingAddProcessForTokensPartition(t *testing.T) {
	// create fee manager for money partition
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
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
		},
	}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// create fee context with token partition id
	feeCtx := &AddFeeCreditCtx{
		TargetPartitionID:  tokensSystemID,
		TargetBillID:       []byte{2},
		TargetBillBacklink: []byte{2},
	}
	err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, feeCtx)
	require.NoError(t, err)

	// when attempting to add fees for money partition
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})

	// then error must be returned
	require.ErrorIs(t, err, ErrInvalidPartition)
	require.Nil(t, res)

	// and feeCtx is not deleted
	actualFeeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
	require.NoError(t, err)
	require.EqualValues(t, feeCtx, actualFeeCtx)
}

func TestReclaimFeeCreditForMoneyPartition_ExistingReclaimProcessForTokensPartition(t *testing.T) {
	// create fee manager for money partition
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// create fee context with token partition id
	feeCtx := &ReclaimFeeCreditCtx{
		TargetPartitionID:  tokensSystemID,
		TargetBillID:       []byte{2},
		TargetBillBacklink: []byte{2},
	}
	err = feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, feeCtx)
	require.NoError(t, err)

	// when attempting to reclaim fees for money partition
	res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})

	// then error must be returned
	require.ErrorIs(t, err, ErrInvalidPartition)
	require.Nil(t, res)

	// and money fee context is not deleted
	actualFeeCtx, err := feeManagerDB.GetReclaimFeeContext(accountKey.PubKey)
	require.NoError(t, err)
	require.Equal(t, feeCtx, actualFeeCtx)
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
				Locked: unitlock.LockReasonCollectDust,
			},
		}}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// verify that the non-locked bill can be reclaimed
	res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Proofs)
	require.NotNil(t, res.Proofs.Lock)
	require.NotNil(t, res.Proofs.CloseFC)
	require.NotNil(t, res.Proofs.ReclaimFC)

	var attr *transactions.CloseFeeCreditAttributes
	require.NoError(t, res.Proofs.CloseFC.TxRecord.TransactionOrder.UnmarshalAttributes(&attr))
	require.Equal(t, []byte{1}, attr.TargetUnitID)
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
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

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
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestAddWithInsufficientBalanceInSmallBills(t *testing.T) {
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
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 4})
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestAddFeeCredit_FeeCreditRecordIsLocked(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{
		bills: []*wallet.Bill{
			{Id: []byte{1}, Value: 100, TxHash: []byte{2}},
		},
		fcb: &wallet.Bill{Id: []byte{1}, Value: 100, Locked: wallet.LockReasonManual},
	}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	// add fees
	addRes, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 10})
	require.ErrorContains(t, err, "fee credit bill is locked")
	require.Nil(t, addRes)

	// reclaim fees
	recRes, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.ErrorContains(t, err, "fee credit bill is locked")
	require.Nil(t, recRes)
}

/*
Fee manager contains LockFC ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => send lockFC
3. if timed out => create new lockFC
*/
func TestAddFeeCredit_ExistingLockFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	feeManagerDB := createFeeManagerDB(t)

	lockFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewLockFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	lockFCProof := &wallet.Proof{TxRecord: lockFCRecord, TxProof: &types.TxProof{}}

	t.Run("lockFC confirmed => send follow-up transactions", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       []byte{1},
			TargetBillBacklink: []byte{200},
			TargetAmount:       50,
			LockFCTx:           lockFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{string(lockFCRecord.TransactionOrder.UnitID()): lockFCProof},
		}

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)

		// then follow-up transactions are sent
		require.NotNil(t, res)
		require.Len(t, res.Proofs, 1)
		proofs := res.Proofs[0]
		require.NotNil(t, proofs.LockFC)
		require.NotNil(t, proofs.TransferFC)
		require.NotNil(t, proofs.AddFC)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("lockFC timed out => create new lockFC and send follow-up transactions", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       []byte{1},
			TargetBillBacklink: []byte{200},
			LockFCTx:           lockFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockFCRecord.TransactionOrder.Timeout() + 10,
			fcb:         &wallet.Bill{Id: []byte{2}, Value: 100, TxHash: []byte{3}},
		}

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Len(t, res.Proofs, 1)
		proofs := res.Proofs[0]
		require.NotNil(t, proofs.LockFC)
		require.NotNil(t, proofs.TransferFC)
		require.NotNil(t, proofs.AddFC)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

/*
Fee manager contains TransferFC ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => send addFC using the confirmed transferFC
3. if timed out and unit still valid => create new transferFC
4. if timed out and unit no longer valid => return error, unlock units
*/
func TestAddFeeCredit_ExistingTransferFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	feeManagerDB := createFeeManagerDB(t)

	transferFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	transferFCProof := &wallet.Proof{TxRecord: transferFCRecord, TxProof: &types.TxProof{}}

	t.Run("transferFC confirmed => send addFC using the confirmed transferFC", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       transferFCRecord.TransactionOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			TransferFCTx:       transferFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{string(transferFCRecord.TransactionOrder.UnitID()): transferFCProof},
		}

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)

		// then addFC tx must be sent using the confirmed transferFC
		require.NotNil(t, res)
		require.Len(t, res.Proofs, 1)
		proofs := res.Proofs[0]
		require.NotNil(t, proofs.TransferFC)
		require.NotNil(t, proofs.AddFC)

		sentAddFCAttr := &transactions.AddFeeCreditAttributes{}
		err = proofs.AddFC.TxRecord.TransactionOrder.UnmarshalAttributes(sentAddFCAttr)
		require.NoError(t, err)
		require.Equal(t, transferFCRecord, sentAddFCAttr.FeeCreditTransfer)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("transferFC timed out => create new transferFC", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       transferFCRecord.TransactionOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			TransferFCTx:       transferFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and the same bill used for transferFC is still valid
		*moneyBackendClient = mockMoneyClient{
			roundNumber: transferFCRecord.TransactionOrder.Timeout() + 10,
			bills: []*wallet.Bill{{
				Id:     transferFCRecord.TransactionOrder.UnitID(),
				Value:  50,
				TxHash: []byte{200},
			}},
		}

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Len(t, res.Proofs, 1)
		proofs := res.Proofs[0]
		require.NotNil(t, proofs.TransferFC)
		require.NotNil(t, proofs.AddFC)

		// then new transferFC must be sent (same id, new timeout)
		require.EqualValues(t, transferFCRecord.TransactionOrder.UnitID(), proofs.TransferFC.TxRecord.TransactionOrder.UnitID())
		require.EqualValues(t, moneyBackendClient.roundNumber+10, proofs.TransferFC.TxRecord.TransactionOrder.Timeout())

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("transferFC timed out and target unit no longer valid => return error", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       transferFCRecord.TransactionOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			TransferFCTx:       transferFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and transferFC unit no longer exists
		*moneyBackendClient = mockMoneyClient{
			roundNumber: transferFCRecord.TransactionOrder.Timeout(),
		}

		// when fees are added then error must be returned
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.Errorf(t, err, "transferFC target unit is no longer valid")
		require.Nil(t, res)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

/*
Fee manager contains AddFC ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => send addFC using the confirmed transferFC
3. if timed out and transferFC still usable => create new addFC
3. if timed out and transferFC no longer usable => return money lost error
*/
func TestAddFeeCredit_ExistingAddFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	addFCAttr := testutils.NewAddFCAttr(t, signer)
	addFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewAddFC(t, signer, addFCAttr,
			testtransaction.WithClientMetadata(&types.ClientMetadata{Timeout: 5, MaxTransactionFee: 2})),
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}
	addFCProof := &wallet.Proof{TxRecord: addFCRecord, TxProof: &types.TxProof{}}

	t.Run("addFC confirmed => return no error (and optionally the fee txs)", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       addFCRecord.TransactionOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			TransferFCTx:       addFCAttr.FeeCreditTransfer.TransactionOrder,
			TransferFCProof:    &wallet.Proof{TxRecord: addFCAttr.FeeCreditTransfer, TxProof: addFCAttr.FeeCreditTransferProof},
			AddFCTx:            addFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{(string)(addFCRecord.TransactionOrder.UnitID()): addFCProof},
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)

		// then no error must be returned (and optionally the fee txs can be returned)
		require.NotNil(t, res)

		// and fee context must be cleared
		lockedBill, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("addFC timed out => create new addFC", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       addFCRecord.TransactionOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			TransferFCTx:       addFCAttr.FeeCreditTransfer.TransactionOrder,
			TransferFCProof:    &wallet.Proof{TxRecord: addFCAttr.FeeCreditTransfer, TxProof: addFCAttr.FeeCreditTransferProof},
			AddFCTx:            addFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out
		// tx timeout (5) < round number (6) < latest addition time (10)
		*moneyBackendClient = mockMoneyClient{
			roundNumber: addFCRecord.TransactionOrder.Timeout() + 1,
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs[0].AddFC)

		// then new addFC must be sent using the existing transferFC
		// new addFC has new tx timeout = round number + txTimeoutBlockCount
		require.EqualValues(t, moneyBackendClient.roundNumber+txTimeoutBlockCount, res.Proofs[0].AddFC.TxRecord.TransactionOrder.Timeout())

		// and fee context must be cleared
		lockedBill, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("addFC timed out and transferFC no longer usable => return money lost error", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       addFCRecord.TransactionOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			TransferFCTx:       addFCAttr.FeeCreditTransfer.TransactionOrder,
			TransferFCProof:    &wallet.Proof{TxRecord: addFCAttr.FeeCreditTransfer, TxProof: addFCAttr.FeeCreditTransferProof},
			AddFCTx:            addFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out
		// round number > latest addition time
		*moneyBackendClient = mockMoneyClient{
			roundNumber: 11,
		}

		// when fees are added
		// then money lost error must be returned
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.ErrorContains(t, err, "addFC timed out and transferFC latestAdditionTime exceeded, the target bill is no longer usable")
		require.Nil(t, res)
	})
}

/*
Fee manager contains Lock ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => send lock tx
3. if timed out => create new lock tx
*/
func TestReclaimFeeCredit_ExistingLock(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	feeManagerDB := createFeeManagerDB(t)

	lockRecord := &types.TransactionRecord{
		TransactionOrder: testtransaction.NewTransactionOrder(t, testtransaction.WithPayloadType(money.PayloadTypeLock)),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	lockProof := &wallet.Proof{TxRecord: lockRecord, TxProof: &types.TxProof{}}

	t.Run("lock tx confirmed => send follow-up transactions", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       []byte{1},
			TargetBillBacklink: []byte{200},
			LockTx:             lockRecord.TransactionOrder,
			LockTxProof:        lockProof,
		})
		require.NoError(t, err)

		// mock locked fee credit bill
		*moneyBackendClient = mockMoneyClient{
			fcb: &wallet.Bill{Id: []byte{1}, Value: 100, TxHash: []byte{200}, Locked: unitlock.LockReasonReclaimFees},
		}

		// when fees are reclaimed
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)

		// then follow-up transactions are sent
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs, 1)
		require.NotNil(t, res.Proofs.Lock)
		require.NotNil(t, res.Proofs.CloseFC)
		require.NotNil(t, res.Proofs.ReclaimFC)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("lock tx timed out => create new lock tx and send follow-up transactions", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       []byte{1},
			TargetBillBacklink: []byte{200},
			LockTx:             lockRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockRecord.TransactionOrder.Timeout() + 10,
			fcb:         &wallet.Bill{Id: []byte{1}, Value: 100, TxHash: []byte{200}},
		}

		// when fees are reclaimed
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs)
		require.NotNil(t, res.Proofs.Lock)
		require.NotNil(t, res.Proofs.CloseFC)
		require.NotNil(t, res.Proofs.ReclaimFC)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

/*
Fee manager contains CloseFC ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => send reclaimFC using the confirmed closeFC
3. if timed out => create new closeFC and reclaimFC
*/
func TestReclaimFeeCredit_ExistingCloseFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))

	closeFCAttr := testutils.NewCloseFCAttr()
	closeFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewCloseFC(t, closeFCAttr),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	closeFCProof := &wallet.Proof{TxRecord: closeFCRecord, TxProof: &types.TxProof{}}

	t.Run("closeFC confirmed => send reclaimFC using the confirmed closeFC", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       closeFCRecord.TransactionOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			CloseFCTx:          closeFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{string(closeFCRecord.TransactionOrder.UnitID()): closeFCProof},
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)

		// then reclaimFC tx must be sent using the confirmed closeFC
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs)
		require.NotNil(t, res.Proofs.CloseFC)
		require.NotNil(t, res.Proofs.ReclaimFC)

		sentReclaimFCAttr := &transactions.ReclaimFeeCreditAttributes{}
		err = res.Proofs.ReclaimFC.TxRecord.TransactionOrder.UnmarshalAttributes(sentReclaimFCAttr)
		require.NoError(t, err)
		require.Equal(t, closeFCRecord, sentReclaimFCAttr.CloseFeeCreditTransfer)

		// and fee context must be cleared
		lockedBill, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("closeFC timed out => create new closeFC", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       closeFCRecord.TransactionOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			CloseFCTx:          closeFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and add bill to wallet
		*moneyBackendClient = mockMoneyClient{
			fcb:         &wallet.Bill{Value: 1e8, Id: closeFCRecord.TransactionOrder.UnitID(), TxHash: []byte{100}},
			roundNumber: closeFCRecord.TransactionOrder.Timeout() + 10,
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs)
		require.NotNil(t, res.Proofs.CloseFC)
		require.NotNil(t, res.Proofs.ReclaimFC)

		// then new closeFC must be sent (same id but timeout changed)
		require.Equal(t, closeFCRecord.TransactionOrder.UnitID(), res.Proofs.CloseFC.TxRecord.TransactionOrder.UnitID())
		require.Equal(t, moneyBackendClient.roundNumber+10, res.Proofs.CloseFC.TxRecord.TransactionOrder.Timeout())

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

/*
Fee manager contains ReclaimFC ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => ok
3. if partially timed out => create new tx (target bill still usable)
4. if fully timed out => return money lost error (target bill has been used)
*/
func TestReclaimFeeCredit_ExistingReclaimFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	reclaimFCAttr := testutils.NewReclaimFCAttr(t, signer)
	reclaimFCOrder := testutils.NewReclaimFC(t, signer, reclaimFCAttr)
	reclaimFCRecord := &types.TransactionRecord{
		TransactionOrder: reclaimFCOrder,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	reclaimFCProof := &wallet.Proof{TxRecord: reclaimFCRecord, TxProof: &types.TxProof{}}

	t.Run("reclaimFC confirmed => return proofs", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       reclaimFCOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			CloseFCTx:          reclaimFCAttr.CloseFeeCreditTransfer.TransactionOrder,
			CloseFCProof:       &wallet.Proof{TxRecord: reclaimFCAttr.CloseFeeCreditTransfer, TxProof: reclaimFCAttr.CloseFeeCreditProof},
			ReclaimFCTx:        reclaimFCProof.TxRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{string(reclaimFCOrder.UnitID()): reclaimFCProof},
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)

		// then reclaimFC proof must be returned
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs)
		require.Equal(t, reclaimFCProof, res.Proofs.ReclaimFC)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("reclaimFC timed out => create new reclaimFC", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       reclaimFCOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			CloseFCTx:          reclaimFCAttr.CloseFeeCreditTransfer.TransactionOrder,
			CloseFCProof:       &wallet.Proof{TxRecord: reclaimFCAttr.CloseFeeCreditTransfer, TxProof: reclaimFCAttr.CloseFeeCreditProof},
			ReclaimFCTx:        reclaimFCProof.TxRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and return locked bill
		*moneyBackendClient = mockMoneyClient{
			roundNumber: reclaimFCOrder.Timeout() + 1,
			bills:       []*wallet.Bill{{Id: reclaimFCOrder.UnitID(), TxHash: []byte{200}}},
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs.ReclaimFC)

		// then new reclaimFC must be sent using the existing closeFC
		// new reclaimFC has new tx timeout = round number + tx timeout
		require.EqualValues(t, moneyBackendClient.roundNumber+txTimeoutBlockCount, res.Proofs.ReclaimFC.TxRecord.TransactionOrder.Timeout())

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("reclaimFC timed out and closeFC no longer usable => return money lost error", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID:  moneySystemID,
			TargetBillID:       reclaimFCOrder.UnitID(),
			TargetBillBacklink: []byte{200},
			CloseFCTx:          reclaimFCAttr.CloseFeeCreditTransfer.TransactionOrder,
			CloseFCProof:       &wallet.Proof{TxRecord: reclaimFCAttr.CloseFeeCreditTransfer, TxProof: reclaimFCAttr.CloseFeeCreditProof},
			ReclaimFCTx:        reclaimFCProof.TxRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and no bills are available
		*moneyBackendClient = mockMoneyClient{
			roundNumber: 11,
		}

		// when fees are reclaimed
		// then money lost error must be returned
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.ErrorContains(t, err, "reclaimFC target bill is no longer usable")
		require.Nil(t, res)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

func TestLockFeeCredit(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	feeManagerDB := createFeeManagerDB(t)

	lockFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewLockFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}

	t.Run("lockFC => ok", func(t *testing.T) {
		// fcb exists
		*moneyBackendClient = mockMoneyClient{
			fcb: &wallet.Bill{Id: lockFCRecord.TransactionOrder.UnitID()},
		}

		// when fee credit is successfully locked then lockFC proof should be returned
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.LockFeeCredit(context.Background(), LockFeeCreditCmd{LockStatus: wallet.LockReasonManual})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, transactions.PayloadTypeLockFeeCredit, res.TxRecord.TransactionOrder.PayloadType())
	})

	t.Run("lockFC => fcb already locked", func(t *testing.T) {
		// fcb already locked
		*moneyBackendClient = mockMoneyClient{
			fcb: &wallet.Bill{Id: lockFCRecord.TransactionOrder.UnitID(), Locked: wallet.LockReasonManual},
		}

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.LockFeeCredit(context.Background(), LockFeeCreditCmd{LockStatus: wallet.LockReasonManual})
		require.ErrorContains(t, err, "fee credit bill is already locked")
		require.Nil(t, res)
	})
}

func TestUnlockFeeCredit(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	feeManagerDB := createFeeManagerDB(t)

	unlockFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewUnlockFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}

	t.Run("unlockFC => ok", func(t *testing.T) {
		// locked fcb exists
		*moneyBackendClient = mockMoneyClient{
			fcb: &wallet.Bill{Id: unlockFCRecord.TransactionOrder.UnitID(), Locked: wallet.LockReasonManual},
		}

		// when fee credit is successfully unlocked then unlockFC proof should be returned
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.UnlockFeeCredit(context.Background(), UnlockFeeCreditCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, transactions.PayloadTypeUnlockFeeCredit, res.TxRecord.TransactionOrder.PayloadType())
	})

	t.Run("unlockFC => fcb already unlocked", func(t *testing.T) {
		// mock fcb already locked
		*moneyBackendClient = mockMoneyClient{
			fcb: &wallet.Bill{Id: unlockFCRecord.TransactionOrder.UnitID()},
		}

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyTxPublisher, moneyBackendClient, logger.New(t))
		res, err := feeManager.UnlockFeeCredit(context.Background(), UnlockFeeCreditCmd{})
		require.ErrorContains(t, err, "fee credit bill is already unlocked")
		require.Nil(t, res)
	})
}

func newMoneyPartitionFeeManager(am account.Manager, db FeeManagerDB, moneyTxPublisher TxPublisher, moneyBackendClient MoneyClient, log *slog.Logger) *FeeManager {
	return NewFeeManager(am, db, moneySystemID, moneyTxPublisher, moneyBackendClient, testFeeCreditRecordIDFromPublicKey, moneySystemID, moneyTxPublisher, moneyBackendClient, testFeeCreditRecordIDFromPublicKey, log)
}

func newAccountManager(t *testing.T) account.Manager {
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	t.Cleanup(am.Close)
	err = am.CreateKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	return am
}

func createFeeManagerDB(t *testing.T) *BoltStore {
	feeManagerDB, err := NewFeeManagerDB(t.TempDir())
	require.NoError(t, err)
	return feeManagerDB
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
