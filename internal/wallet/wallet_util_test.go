package wallet

import (
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
)

type mockAlphaBillClient struct {
}

func (c *mockAlphaBillClient) InitBlockReceiver(blockHeight uint64, ch chan<- *alphabill.Block) error {
	return nil
}

func (c *mockAlphaBillClient) SendTransaction(tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	return &transaction.TransactionResponse{Ok: true}, nil
}

func (c *mockAlphaBillClient) Shutdown() {
	// do nothing
}

func (c *mockAlphaBillClient) IsShutdown() bool {
	return false
}

func DeleteWallet(w *Wallet) {
	if w != nil {
		w.Shutdown()
		w.DeleteDb()
	}
}
