package money

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/stretchr/testify/require"
)

func CreateTestWallet(t *testing.T) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)

	w, err := CreateNewWallet(am, "", WalletConfig{DbPath: dir})
	require.NoError(t, err)
	t.Cleanup(w.Shutdown)

	mockClient := clientmock.NewMockAlphabillClient(0, map[uint64]*block.Block{})
	w.AlphabillClient = mockClient
	return w, mockClient
}

func CreateTestWalletFromSeed(t *testing.T) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	w, err := CreateNewWallet(am, testMnemonic, WalletConfig{DbPath: dir})
	require.NoError(t, err)
	t.Cleanup(w.Shutdown)

	mockClient := &clientmock.MockAlphabillClient{}
	w.AlphabillClient = mockClient
	return w, mockClient
}

func CopyWalletDBFile(t *testing.T) (string, error) {
	wd, _ := os.Getwd()
	srcDir := filepath.Join(wd, "testdata", "wallet")
	return copyWalletDB(srcDir, t.TempDir())
}

func copyWalletDB(srcDir string, dstDir string) (string, error) {
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
