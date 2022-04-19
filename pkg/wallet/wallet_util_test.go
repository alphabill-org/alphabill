package wallet

import (
	"os"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutil"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"github.com/stretchr/testify/require"
)

type mockAlphabillClient struct {
	txs        []*transaction.Transaction
	txResponse *transaction.TransactionResponse
	maxBlockNo uint64
	isShutdown bool
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
	return c.maxBlockNo, nil
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
