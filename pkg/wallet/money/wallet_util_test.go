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
	c := WalletConfig{DbPath: t.TempDir()}
	w, err := CreateNewWallet("", c)
	require.NoError(t, err)
	t.Cleanup(w.Shutdown)

	mockClient := clientmock.NewMockAlphabillClient(0, map[uint64]*block.Block{})
	w.AlphabillClient = mockClient
	return w, mockClient
}

func CreateTestWalletFromSeed(t *testing.T) (*Wallet, *clientmock.MockAlphabillClient) {
	w, err := CreateNewWallet(testMnemonic, WalletConfig{DbPath: t.TempDir()})
	require.NoError(t, err)

	mockClient := &clientmock.MockAlphabillClient{}
	w.AlphabillClient = mockClient
	return w, mockClient
}

func CopyWalletDBFile(t *testing.T) (string, error) {
	wd, _ := os.Getwd()
	srcDir := path.Join(wd, "testdata", "wallet")
	return copyWalletDB(srcDir, t.TempDir())
}

func CopyEncryptedWalletDBFile(t *testing.T) (string, error) {
	wd, _ := os.Getwd()
	srcDir := path.Join(wd, "testdata", "wallet", "encrypted")
	return copyWalletDB(srcDir, t.TempDir())
}

func copyWalletDB(srcDir string, dstDir string) (string, error) {
	srcFile := path.Join(srcDir, WalletFileName)
	dstFile := path.Join(dstDir, WalletFileName)
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
