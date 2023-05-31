package fees

import (
	"context"
	"testing"

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
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	defer am.Close()
	err = am.CreateKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)

	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{{
		Id:            []byte{1},
		Value:         100000002,
		TxHash:        []byte{2},
		FcBlockNumber: 3,
	}}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// verify that entire bill amount can be added
	_, err = feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000001})
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
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	defer am.Close()
	err = am.CreateKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)

	moneyTxPublisher := &mockMoneyTxPublisher{}
	moneyBackendClient := &mockMoneyClient{bills: []*wallet.Bill{}}
	feeManager := newMoneyPartitionFeeManager(am, moneyTxPublisher, moneyBackendClient)

	// verify that error is returned
	_, err = feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.ErrorContains(t, err, "wallet does not contain any bills")
}

func newMoneyPartitionFeeManager(am account.Manager, moneyTxPublisher TxPublisher, moneyBackendClient MoneyClient) *FeeManager {
	moneySystemID := []byte{0, 0, 0, 0}
	return NewFeeManager(am, moneySystemID, moneyTxPublisher, moneyBackendClient, moneySystemID, moneyTxPublisher, moneyBackendClient)
}

type mockMoneyClient struct {
	bills []*wallet.Bill
}

func (m *mockMoneyClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (m *mockMoneyClient) GetBills(pubKey []byte) ([]*wallet.Bill, error) {
	return m.bills, nil
}

func (m *mockMoneyClient) FetchFeeCreditBill(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
	return nil, nil
}

type mockMoneyTxPublisher struct {
}

func (m *mockMoneyTxPublisher) SendTx(ctx context.Context, tx *types.TransactionOrder, _ []byte) (*wallet.Proof, error) {
	return &wallet.Proof{}, nil
}
