package money

import (
	"os"
	"path"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

type mockAlphabillClient struct {
	txs        []*txsystem.Transaction
	txResponse *txsystem.TransactionResponse
	maxBlockNo uint64
	shutdown   bool
}

func (c *mockAlphabillClient) SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	c.txs = append(c.txs, tx)
	if c.txResponse != nil {
		return c.txResponse, nil
	}
	return &txsystem.TransactionResponse{Ok: true}, nil
}

func (c *mockAlphabillClient) GetBlock(uint64) (*block.Block, error) {
	return nil, nil
}

func (c *mockAlphabillClient) GetMaxBlockNumber() (uint64, error) {
	return c.maxBlockNo, nil
}

func (c *mockAlphabillClient) Shutdown() {
	c.shutdown = true
}

func (c *mockAlphabillClient) IsShutdown() bool {
	return c.shutdown
}

func CreateTestWallet(t *testing.T) (*Wallet, *mockAlphabillClient) {
	_ = DeleteWalletDb(os.TempDir())
	c := WalletConfig{DbPath: os.TempDir()}
	w, err := CreateNewWallet("", c)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	mockClient := &mockAlphabillClient{}
	w.AlphabillClient = mockClient
	return w, mockClient
}

func CreateTestWalletFromSeed(t *testing.T) (*Wallet, *mockAlphabillClient) {
	_ = DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet(testMnemonic, WalletConfig{DbPath: os.TempDir()})
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	mockClient := &mockAlphabillClient{}
	w.AlphabillClient = mockClient
	return w, mockClient
}

func DeleteWallet(w *Wallet) {
	if w != nil {
		w.Shutdown()
		w.DeleteDb()
	}
}

func DeleteWalletDb(walletDir string) error {
	dbFilePath := path.Join(walletDir, walletFileName)
	return os.Remove(dbFilePath)
}

func CopyWalletDBFile(t *testing.T) (string, error) {
	_ = DeleteWalletDb(os.TempDir())
	t.Cleanup(func() {
		_ = DeleteWalletDb(os.TempDir())
	})
	wd, _ := os.Getwd()
	srcDir := path.Join(wd, "testdata", "wallet")
	return copyWalletDB(srcDir)
}

func CopyEncryptedWalletDBFile(t *testing.T) (string, error) {
	_ = DeleteWalletDb(os.TempDir())
	t.Cleanup(func() {
		_ = DeleteWalletDb(os.TempDir())
	})
	wd, _ := os.Getwd()
	srcDir := path.Join(wd, "testdata", "wallet", "encrypted")
	return copyWalletDB(srcDir)
}

func copyWalletDB(srcDir string) (string, error) {
	dstDir := os.TempDir()
	srcFile := path.Join(srcDir, walletFileName)
	dstFile := path.Join(dstDir, walletFileName)
	return dstDir, copyFile(srcFile, dstFile)
}

func copyFile(src string, dst string) error {
	srcBytes, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	err = os.WriteFile(dst, srcBytes, 0700)
	if err != nil {
		return err
	}
	return nil
}
