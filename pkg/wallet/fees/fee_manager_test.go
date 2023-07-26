package fees

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/stretchr/testify/require"
)

/*
Test scenario:
Wallet has single bill with value 1.00000002
Add fee credit with value 1.00000001
Result should have 0 balance, 1.00000000 in fee credit and no error and any larger amount should return error.
*/
func TestAddFeeCredit_SpendEntireBill_OK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{{
		Id:     []byte{1},
		Value:  100000002,
		TxHash: []byte{2},
	}}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// verify that entire bill amount can be added
	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000001})
	require.NoError(t, err)

	// verify that any larger amount returns error
	_, err = feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000002})
	require.ErrorContains(t, err, "wallet does not have a bill large enough for fee transfer")
}

/*
Test scenario:
Wallet has no bills
Trying to create fee credit should return error "wallet does not contain any bills"
*/
func TestAddFeeCredit_NoBillsReturnsError(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// verify that error is returned
	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.ErrorContains(t, err, "wallet does not contain any bills")
}

func TestAddFeeCredit_LockedFeeCredit_OK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	lfc := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: lfc, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{lockedFeeCredit: lfc, proof: txrProof}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when add fee credit is called
	proofs, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
	require.NoError(t, err)

	// then addFC tx must be sent using the existing transferFC
	require.NotNil(t, proofs)
	require.Nil(t, proofs.TransferFC)
	require.NotNil(t, proofs.AddFC)
	require.Equal(t, transactions.PayloadTypeAddFeeCredit, proofs.AddFC.TxRecord.TransactionOrder.PayloadType())
	actualAttr := &transactions.AddFeeCreditAttributes{}
	err = proofs.AddFC.TxRecord.TransactionOrder.UnmarshalAttributes(actualAttr)
	require.NoError(t, err)
	require.Equal(t, lfc, actualAttr.FeeCreditTransfer)
}

func TestAddFeeCredit_LockedFeeCredit_InvalidTimeout(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	txr := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: txr, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{lockedFeeCredit: txr, proof: txrProof, roundNumber: 10, bills: []*wallet.Bill{{
		Id:     []byte{1},
		Value:  100000002,
		TxHash: []byte{2},
	}}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when add fee credit is called when round number equals to the latest allowed block in transferFC
	proofs, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
	require.NoError(t, err)

	// then new transferFC and addFC must be sent
	require.NotNil(t, proofs)
	require.NotNil(t, proofs.TransferFC)
	require.NotNil(t, proofs.AddFC)
	require.Equal(t, transactions.PayloadTypeTransferFeeCredit, proofs.TransferFC.TxRecord.TransactionOrder.PayloadType())
	require.Equal(t, transactions.PayloadTypeAddFeeCredit, proofs.AddFC.TxRecord.TransactionOrder.PayloadType())
}

func TestAddFeeCredit_LockedFeeCredit_InvalidAmount(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	lfc := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: lfc, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{lockedFeeCredit: lfc, proof: txrProof}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when add fee credit is called with incorrect amount
	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 51})
	require.Errorf(t, err, "invalid amount: locked fee credit exists for amount %d but user specified %d", 50, 51)
}

func TestAddFeeCredit_ClosedFeeCredit_OK(t *testing.T) {
	// create fee manager with existing closeFC tx
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	closeFCAttr := testutils.NewCloseFCAttr(
		testutils.WithCloseFCTargetUnitID([]byte{1}),
		testutils.WithCloseFCAmount(50),
		testutils.WithCloseFCNonce([]byte{3}),
	)
	cfc := &types.TransactionRecord{
		TransactionOrder: testutils.NewCloseFC(t, closeFCAttr),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: cfc, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{closedFeeCredit: cfc, proof: txrProof, bills: []*wallet.Bill{{
		Id:     closeFCAttr.TargetUnitID,
		Value:  closeFCAttr.Amount,
		TxHash: closeFCAttr.Nonce,
	}}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when reclaim fee credit is called with existing closeFC tx
	proofs, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.NoError(t, err)

	// then reclaimFC tx must be sent using the existing closeFC tx
	require.NotNil(t, proofs)
	require.Nil(t, proofs.CloseFC)
	require.NotNil(t, proofs.ReclaimFC)
	require.Equal(t, transactions.PayloadTypeReclaimFeeCredit, proofs.ReclaimFC.TxRecord.TransactionOrder.PayloadType())
	actualAttr := &transactions.ReclaimFeeCreditAttributes{}
	err = proofs.ReclaimFC.TxRecord.TransactionOrder.UnmarshalAttributes(actualAttr)
	require.NoError(t, err)
	require.Equal(t, cfc, actualAttr.CloseFeeCreditTransfer)
}

func TestAddFeeCredit_ClosedFeeCredit_InvalidTargetUnitID(t *testing.T) {
	// create fee manager with existing closeFC tx
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	closeFCAttr := testutils.NewCloseFCAttr(
		testutils.WithCloseFCTargetUnitID([]byte{1}),
		testutils.WithCloseFCAmount(50),
		testutils.WithCloseFCNonce([]byte{3}),
	)
	cfc := &types.TransactionRecord{
		TransactionOrder: testutils.NewCloseFC(t, closeFCAttr),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: cfc, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{closedFeeCredit: cfc, proof: txrProof, bills: []*wallet.Bill{{
		Id:     []byte{2}, // bill for closeFC.TargetUnitID does not exist
		Value:  closeFCAttr.Amount,
		TxHash: closeFCAttr.Nonce,
	}}, fcb: &wallet.Bill{Value: 1}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when reclaim fee credit is called with existing closeFC tx but invalid ID
	proofs, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.NoError(t, err)

	// then new closeFC tx must be created
	require.NotNil(t, proofs)
	require.NotNil(t, proofs.CloseFC)
	require.NotNil(t, proofs.ReclaimFC)
	require.Equal(t, transactions.PayloadTypeCloseFeeCredit, proofs.CloseFC.TxRecord.TransactionOrder.PayloadType())
	require.Equal(t, transactions.PayloadTypeReclaimFeeCredit, proofs.ReclaimFC.TxRecord.TransactionOrder.PayloadType())
}

func TestAddFeeCredit_ClosedFeeCredit_InvalidTargetTxHash(t *testing.T) {
	// create fee manager with existing closeFC tx
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	closeFCAttr := testutils.NewCloseFCAttr(
		testutils.WithCloseFCTargetUnitID([]byte{1}),
		testutils.WithCloseFCAmount(50),
		testutils.WithCloseFCNonce([]byte{3}),
	)
	cfc := &types.TransactionRecord{
		TransactionOrder: testutils.NewCloseFC(t, closeFCAttr),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: cfc, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{closedFeeCredit: cfc, proof: txrProof, bills: []*wallet.Bill{{
		Id:     closeFCAttr.TargetUnitID,
		Value:  closeFCAttr.Amount,
		TxHash: []byte{4}, // bill for closeFC.TargetUnitID does not exist
	}}, fcb: &wallet.Bill{Value: 1}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when reclaim fee credit is called with existing closeFC tx but invalid txHash (e.g. target bill has been used)
	proofs, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.NoError(t, err)

	// then new closeFC tx must be created
	require.NotNil(t, proofs)
	require.NotNil(t, proofs.CloseFC)
	require.NotNil(t, proofs.ReclaimFC)
	require.Equal(t, transactions.PayloadTypeCloseFeeCredit, proofs.CloseFC.TxRecord.TransactionOrder.PayloadType())
	require.Equal(t, transactions.PayloadTypeReclaimFeeCredit, proofs.ReclaimFC.TxRecord.TransactionOrder.PayloadType())
}

func TestAddFeeCredit_CannotAddFeesWhenUnreclaimedFeesExist(t *testing.T) {
	// create fee manager with existing closeFC tx
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	closeFCAttr := testutils.NewCloseFCAttr(
		testutils.WithCloseFCTargetUnitID([]byte{1}),
		testutils.WithCloseFCAmount(50),
		testutils.WithCloseFCNonce([]byte{3}),
	)
	cfc := &types.TransactionRecord{
		TransactionOrder: testutils.NewCloseFC(t, closeFCAttr),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: cfc, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{closedFeeCredit: cfc, proof: txrProof, bills: []*wallet.Bill{{
		Id:     closeFCAttr.TargetUnitID,
		Value:  closeFCAttr.Amount,
		TxHash: closeFCAttr.Nonce,
	}}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when add fee credit is called with existing closeFC tx
	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{})
	require.Errorf(t, err, "wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit")
}

func newMoneyPartitionFeeManager(am account.Manager, moneyTxPublisher TxPublisher, moneyBackendClient MoneyClient) *FeeManager {
	moneySystemID := []byte{0, 0, 0, 0}
	return NewFeeManager(am, moneySystemID, moneyTxPublisher, moneyBackendClient, moneySystemID, moneyTxPublisher, moneyBackendClient)
}

func newAccountManager(t *testing.T) account.Manager {
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	t.Cleanup(am.Close)
	err = am.CreateKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	return am
}

type mockMoneyClient struct {
	bills           []*wallet.Bill
	lockedFeeCredit *types.TransactionRecord
	closedFeeCredit *types.TransactionRecord
	proof           *wallet.Proof
	roundNumber     uint64
	fcb             *wallet.Bill
}

func (m *mockMoneyClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	return m.roundNumber, nil
}

func (m *mockMoneyClient) GetBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error) {
	return m.bills, nil
}

func (m *mockMoneyClient) GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*wallet.Bill, error) {
	return m.fcb, nil
}

func (m *mockMoneyClient) GetLockedFeeCredit(ctx context.Context, unitID []byte, fcbID []byte) (*types.TransactionRecord, error) {
	return m.lockedFeeCredit, nil
}

func (m *mockMoneyClient) GetClosedFeeCredit(ctx context.Context, fcbID []byte) (*types.TransactionRecord, error) {
	return m.closedFeeCredit, nil
}

func (m *mockMoneyClient) GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	return m.proof, nil
}

type mockMoneyTxPublisher struct {
	sendTxCount int
}

func (m *mockMoneyTxPublisher) SendTx(ctx context.Context, tx *types.TransactionOrder, _ []byte) (*wallet.Proof, error) {
	m.sendTxCount++
	return &wallet.Proof{TxRecord: &types.TransactionRecord{
		TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}},
	}, nil
}

func (m *mockMoneyTxPublisher) Close() {
}
