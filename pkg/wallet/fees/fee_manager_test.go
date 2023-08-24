package fees

import (
	"context"
	"crypto"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
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
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient)

	// verify that entire bill amount can be added
	proofs, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000001})
	require.NoError(t, err)
	require.NotNil(t, proofs)
	require.NotNil(t, proofs.TransferFC)
	require.NotNil(t, proofs.AddFC)
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
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient)

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
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{}}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient)

	// lock bill with ReasonReclaimFees
	err := unitLocker.LockUnit(&unitlock.LockedUnit{
		UnitID:     []byte{1},
		LockReason: unitlock.ReasonReclaimFees,
	})
	require.NoError(t, err)

	// verify error is returned
	_, err = feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.ErrorContains(t, err, "wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit")
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
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient)

	transferFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	transferFCProof := &wallet.Proof{TxRecord: transferFCRecord, TxProof: &types.TxProof{}}
	lockedTransferFCBill := &unitlock.LockedUnit{
		UnitID:     transferFCRecord.TransactionOrder.UnitID(),
		LockReason: unitlock.ReasonAddFees,
		Transactions: []*unitlock.Transaction{{
			TxOrder:     transferFCRecord.TransactionOrder,
			TxHash:      transferFCRecord.TransactionOrder.Hash(crypto.SHA256),
			PayloadType: transactions.PayloadTypeTransferFeeCredit,
			Timeout:     10, // same as latest addition time on transferFC tx
		}},
	}

	t.Run("transferFC confirmed => send addFC using the confirmed transferFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedTransferFCBill)
		require.NoError(t, err)

		// mock tx confirmed on backend
		*moneyBackendClient = mockMoneyClient{
			proofs: map[string]*wallet.Proof{string(lockedTransferFCBill.UnitID): transferFCProof},
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)

		// then addFC tx must be sent using the confirmed transferFC
		require.NotNil(t, res)
		require.NotNil(t, res.TransferFC)
		require.NotNil(t, res.AddFC)

		sentAddFCAttr := &transactions.AddFeeCreditAttributes{}
		err = res.AddFC.TxRecord.TransactionOrder.UnmarshalAttributes(sentAddFCAttr)
		require.NoError(t, err)
		require.Equal(t, transferFCRecord, sentAddFCAttr.FeeCreditTransfer)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedTransferFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("transferFC timed out => create new transferFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedTransferFCBill)
		require.NoError(t, err)

		// mock tx timed out and add bill to wallet
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedTransferFCBill.Transactions[0].Timeout,
			bills: []*wallet.Bill{{
				Id:     []byte{123},
				Value:  100,
				TxHash: []byte{2},
			}},
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.TransferFC)
		require.NotNil(t, res.AddFC)

		// then new transferFC must be sent
		require.Equal(t, []byte{123}, res.TransferFC.TxRecord.TransactionOrder.UnitID())

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedTransferFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("transferFC still pending => re-send the transferFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedTransferFCBill)
		require.NoError(t, err)

		// mock tx not yet timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedTransferFCBill.Transactions[0].Timeout - 1,
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)

		// then the pending transferFC must be re-sent
		require.Equal(t, transferFCProof, res.TransferFC)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedTransferFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
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
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient)
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	addFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewAddFC(t, signer, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	addFCProof := &wallet.Proof{TxRecord: addFCRecord, TxProof: &types.TxProof{}}
	lockedAddFCBill := &unitlock.LockedUnit{
		UnitID:     addFCRecord.TransactionOrder.UnitID(),
		LockReason: unitlock.ReasonAddFees,
		Transactions: []*unitlock.Transaction{{
			TxOrder:     addFCRecord.TransactionOrder,
			TxHash:      addFCRecord.TransactionOrder.Hash(crypto.SHA256),
			PayloadType: transactions.PayloadTypeAddFeeCredit,
			Timeout:     5, // latest addition time = 10
		}},
	}

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
		lockedBill, err := unitLocker.GetUnit(lockedAddFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("addFC still pending => re-send the addFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedAddFCBill)
		require.NoError(t, err)

		// mock tx timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedAddFCBill.Transactions[0].Timeout - 1,
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res.AddFC)

		// then AddFC must be re-sent
		require.Equal(t, addFCRecord, res.AddFC.TxRecord)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedAddFCBill.UnitID)
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
			roundNumber: lockedAddFCBill.Transactions[0].Timeout + 1,
		}

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.AddFC)

		// then new addFC must be sent using the existing transferFC
		// new addFC has new tx timeout = round number + txTimeoutBlockCount
		require.EqualValues(t, moneyBackendClient.roundNumber+txTimeoutBlockCount, res.AddFC.TxRecord.TransactionOrder.Timeout())

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedAddFCBill.UnitID)
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

/*
Wallet contains locked bill for closeFC and the tx is either:
1. confirmed => send reclaimFC using the confirmed closeFC
2. timed out => create new tx
3. still pending => re-send the tx
*/
func TestReclaimFeeCredit_LockedBillForCloseFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient)

	closeFCRecord := &types.TransactionRecord{
		TransactionOrder: testutils.NewCloseFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	closeFCProof := &wallet.Proof{TxRecord: closeFCRecord, TxProof: &types.TxProof{}}
	lockedCloseFCBill := &unitlock.LockedUnit{
		UnitID:     closeFCRecord.TransactionOrder.UnitID(),
		LockReason: unitlock.ReasonReclaimFees,
		Transactions: []*unitlock.Transaction{{
			TxOrder:     closeFCRecord.TransactionOrder,
			TxHash:      closeFCRecord.TransactionOrder.Hash(crypto.SHA256),
			PayloadType: transactions.PayloadTypeCloseFeeCredit,
			Timeout:     10,
		}},
	}

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
		lockedBill, err := unitLocker.GetUnit(lockedCloseFCBill.UnitID)
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
			roundNumber: lockedCloseFCBill.Transactions[0].Timeout,
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

		require.Equal(t, []byte{111}, res.CloseFC.TxRecord.TransactionOrder.UnitID()) // unitID matches FCB ID
		require.Equal(t, []byte{123}, actualCloseFCAttr.TargetUnitID)                 // target unitID matches target bill ID
		require.Equal(t, []byte{2}, actualCloseFCAttr.Nonce)                          // target nonce matches target bill txhash

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedCloseFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("closeFC still pending => re-send the closeFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedCloseFCBill)
		require.NoError(t, err)

		// mock tx not yet timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedCloseFCBill.Transactions[0].Timeout - 1,
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)

		// then the pending transferFC must be re-sent
		require.Equal(t, closeFCProof, res.CloseFC)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedCloseFCBill.UnitID)
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
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{}
	unitLocker := createUnitLocker(t)
	feeManager := newMoneyPartitionFeeManager(am, unitLocker, moneyTxPublisher, moneyBackendClient)
	signer, _ := abcrypto.NewInMemorySecp256K1Signer()

	reclaimFCOrder := testutils.NewReclaimFC(t, signer, nil)
	reclaimFCRecord := &types.TransactionRecord{
		TransactionOrder: reclaimFCOrder,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	reclaimFCProof := &wallet.Proof{TxRecord: reclaimFCRecord, TxProof: &types.TxProof{}}
	lockedReclaimFCBill := &unitlock.LockedUnit{
		UnitID:     reclaimFCRecord.TransactionOrder.UnitID(),
		LockReason: unitlock.ReasonReclaimFees,
		Transactions: []*unitlock.Transaction{{
			TxOrder:     reclaimFCOrder,
			TxHash:      reclaimFCOrder.Hash(crypto.SHA256),
			PayloadType: transactions.PayloadTypeReclaimFeeCredit,
			Timeout:     10,
		}},
	}

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
		lockedBill, err := unitLocker.GetUnit(reclaimFCOrder.UnitID())
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("reclaimFC still pending => re-send the reclaimFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedReclaimFCBill)
		require.NoError(t, err)

		// mock tx timed out
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedReclaimFCBill.Transactions[0].Timeout - 1,
		}

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res.ReclaimFC)

		// then AddFC must be re-sent
		require.Equal(t, reclaimFCRecord, res.ReclaimFC.TxRecord)

		// and bill must be unlocked
		lockedBill, err := unitLocker.GetUnit(lockedReclaimFCBill.UnitID)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("reclaimFC partially timed out => create new reclaimFC", func(t *testing.T) {
		// lock bill in db
		err := unitLocker.LockUnit(lockedReclaimFCBill)
		require.NoError(t, err)

		// mock tx timed out and return locked bill
		*moneyBackendClient = mockMoneyClient{
			roundNumber: lockedReclaimFCBill.Transactions[0].Timeout + 1,
			bills:       []*wallet.Bill{{Id: lockedReclaimFCBill.UnitID, TxHash: lockedReclaimFCBill.Transactions[0].TxHash}}, // TODO if reclaimFC is timed out we do not have bill available
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
		lockedBill, err := unitLocker.GetUnit(lockedReclaimFCBill.UnitID)
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
		require.ErrorContains(t, err, "reclaimFC target unit hash does not match locked unit hash")
		require.Nil(t, res)
	})
}

func newMoneyPartitionFeeManager(am account.Manager, unitLocker UnitLocker, moneyTxPublisher TxPublisher, moneyBackendClient MoneyClient) *FeeManager {
	moneySystemID := []byte{0, 0, 0, 0}
	return NewFeeManager(am, unitLocker, moneySystemID, moneyTxPublisher, moneyBackendClient, moneySystemID, moneyTxPublisher, moneyBackendClient, money.NewFeeCreditRecordID)
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
