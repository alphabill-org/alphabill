package money

import (
	"os"
	"path"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/stretchr/testify/require"
)

func CreateTestWallet(t *testing.T) (*Wallet, *clientmock.MockAlphabillClient) {
	_ = DeleteWalletDb(os.TempDir())
	c := WalletConfig{DbPath: os.TempDir()}
	w, err := CreateNewWallet("", c)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	mockClient := clientmock.NewMockAlphabillClient(0, map[uint64]*block.Block{})
	w.AlphabillClient = mockClient
	return w, mockClient
}

func CreateTestWalletFromSeed(t *testing.T) (*Wallet, *clientmock.MockAlphabillClient) {
	_ = DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet(testMnemonic, WalletConfig{DbPath: os.TempDir()})
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
