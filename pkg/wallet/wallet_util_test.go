package wallet

import (
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"alphabill-wallet-sdk/internal/testutil"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

type mockAlphaBillClient struct {
	txs        []*transaction.Transaction
	isShutdown bool
	txResponse *transaction.TransactionResponse
}

func (c *mockAlphaBillClient) InitBlockReceiver(blockHeight uint64, ch chan<- *alphabill.GetBlocksResponse) error {
	return nil
}

func (c *mockAlphaBillClient) SendTransaction(tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	c.txs = append(c.txs, tx)
	if c.txResponse != nil {
		return c.txResponse, nil
	}
	return &transaction.TransactionResponse{Ok: true}, nil
}

func (c *mockAlphaBillClient) Shutdown() {
	c.isShutdown = true
}

func (c *mockAlphaBillClient) IsShutdown() bool {
	return c.isShutdown
}

func CreateTestWallet(t *testing.T) (*Wallet, *mockAlphaBillClient) {
	_ = testutil.DeleteWalletDb(os.TempDir())
	c := &Config{DbPath: os.TempDir()}
	w, err := CreateNewWallet(c)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	mockClient := &mockAlphaBillClient{}
	w.alphaBillClient = mockClient
	return w, mockClient
}

func DeleteWallet(w *Wallet) {
	if w != nil {
		w.Shutdown()
		w.DeleteDb()
	}
}
