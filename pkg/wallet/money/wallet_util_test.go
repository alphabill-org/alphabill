package money

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/stretchr/testify/require"
)

func CreateTestWallet(t *testing.T) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)

	w, err := CreateNewWallet(am, "", WalletConfig{DbPath: dir})
	t.Cleanup(func() {
		w.Shutdown()
	})
	require.NoError(t, err)

	mockClient := clientmock.NewMockAlphabillClient()
	w.AlphabillClient = mockClient
	return w, mockClient
}

func CreateTestWalletFromSeed(t *testing.T) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	w, err := CreateNewWallet(am, testMnemonic, WalletConfig{DbPath: dir})
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	mockClient := &clientmock.MockAlphabillClient{}
	w.AlphabillClient = mockClient
	return w, mockClient
}

func DeleteWallet(w *Wallet) {
	if w != nil {
		w.Shutdown()
		w.DeleteDb()
	}
}

func DeleteWalletDbs(walletDir string) error {
	if err := os.Remove(filepath.Join(walletDir, account.AccountFileName)); err != nil {
		return err
	}
	return os.Remove(filepath.Join(walletDir, WalletFileName))
}

func CopyWalletDBFile(t *testing.T) (string, error) {
	_ = DeleteWalletDbs(os.TempDir())
	t.Cleanup(func() {
		_ = DeleteWalletDbs(os.TempDir())
	})
	wd, _ := os.Getwd()
	srcDir := filepath.Join(wd, "testdata", "wallet")
	return copyWalletDB(srcDir)
}

func copyWalletDB(srcDir string) (string, error) {
	dstDir := os.TempDir()
	srcFile := filepath.Join(srcDir, WalletFileName)
	dstFile := filepath.Join(dstDir, WalletFileName)
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
