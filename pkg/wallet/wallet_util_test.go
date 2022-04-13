package wallet

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutil"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

type mockAlphabillClient struct {
	txs        []*transaction.Transaction
	isShutdown bool
	txResponse *transaction.TransactionResponse
}

func (c *mockAlphabillClient) SendTransaction(tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	c.txs = append(c.txs, tx)
	if c.txResponse != nil {
		return c.txResponse, nil
	}
	return &transaction.TransactionResponse{Ok: true}, nil
}

func (c *mockAlphabillClient) GetBlock(blockNo uint64) (*alphabill.Block, error) {
	return nil, nil
}

func (c *mockAlphabillClient) GetMaxBlockNo() (uint64, error) {
	return 0, nil
}

func (c *mockAlphabillClient) Shutdown() {
	c.isShutdown = true
}

func (c *mockAlphabillClient) IsShutdown() bool {
	return c.isShutdown
}

func CreateTestWallet(t *testing.T) (*Wallet, *mockAlphabillClient) {
	_ = testutil.DeleteWalletDb(os.TempDir())
	c := Config{DbPath: os.TempDir()}
	w, err := CreateNewWallet(c)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	mockClient := &mockAlphabillClient{}
	w.alphaBillClient = mockClient
	return w, mockClient
}

func CreateTestWalletFromSeed(t *testing.T) (*Wallet, *mockAlphabillClient) {
	_ = testutil.DeleteWalletDb(os.TempDir())
	w, err := CreateWalletFromSeed(testMnemonic, Config{DbPath: os.TempDir()})
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	mockClient := &mockAlphabillClient{}
	w.alphaBillClient = mockClient
	return w, mockClient
}

func DeleteWallet(w *Wallet) {
	if w != nil {
		w.Shutdown()
		w.DeleteDb()
	}
}
