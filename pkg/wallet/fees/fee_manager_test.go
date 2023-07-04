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

func TestAddFeeCredit_PreviousTransferFC_OK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	lfc := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: lfc, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{lfc: lfc, proof: txrProof}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when add fee credit is called
	proofs, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
	require.NoError(t, err)

	// then addFC tx must be sent using the existing transferFC
	require.Len(t, proofs, 1)
	require.Equal(t, transactions.PayloadTypeAddFeeCredit, proofs[0].TxRecord.TransactionOrder.PayloadType())
	actualAttr := &transactions.AddFeeCreditAttributes{}
	err = proofs[0].TxRecord.TransactionOrder.UnmarshalAttributes(actualAttr)
	require.NoError(t, err)
	require.Equal(t, lfc, actualAttr.FeeCreditTransfer)
}

func TestAddFeeCredit_PreviousTransferFC_InvalidTimeout(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	txr := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: txr, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{lfc: txr, proof: txrProof, roundNumber: 10, bills: []*wallet.Bill{{
		Id:     []byte{1},
		Value:  100000002,
		TxHash: []byte{2},
	}}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when add fee credit is called when round number equals to the latest allowed block in transferFC
	proofs, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
	require.NoError(t, err)

	// then new transferFC and addFC must be sent
	require.Len(t, proofs, 2)
	require.Equal(t, transactions.PayloadTypeTransferFeeCredit, proofs[0].TxRecord.TransactionOrder.PayloadType())
	require.Equal(t, transactions.PayloadTypeAddFeeCredit, proofs[1].TxRecord.TransactionOrder.PayloadType())
}

func TestAddFeeCredit_PreviousTransferFC_InvalidAmount(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyTxPublisher := &mockMoneyTxPublisher{}
	lfc := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	txrProof := &wallet.Proof{TxRecord: lfc, TxProof: &types.TxProof{}}
	moneyBackendClient := &mockMoneyClient{lfc: lfc, proof: txrProof}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// when add fee credit is called with incorrect amount
	_, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 51})
	require.Errorf(t, err, "invalid amount: locked fee credit exists for amount %d but user specified %d", 50, 51)
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
	bills       []*wallet.Bill
	lfc         *types.TransactionRecord
	proof       *wallet.Proof
	roundNumber uint64
}

func (m *mockMoneyClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	return m.roundNumber, nil
}

func (m *mockMoneyClient) GetBills(pubKey []byte) ([]*wallet.Bill, error) {
	return m.bills, nil
}

func (m *mockMoneyClient) GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*wallet.Bill, error) {
	return nil, nil
}

func (m *mockMoneyClient) GetLockedFeeCredit(ctx context.Context, unitID []byte, fcbID []byte) (*types.TransactionRecord, error) {
	return m.lfc, nil
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
