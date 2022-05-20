package money

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const walletPass = "default-wallet-pass"

func TestEncryptedWalletCanBeCreated(t *testing.T) {
	_ = DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet(testMnemonic, WalletConfig{DbPath: os.TempDir(), WalletPass: walletPass})
	t.Cleanup(func() {
		DeleteWallet(w)
	})

	isEncrypted, err := w.db.Do().IsEncrypted()
	require.NoError(t, err)
	require.True(t, isEncrypted)
	verifyTestWallet(t, w)
}

func TestEncryptedWalletCanBeLoaded(t *testing.T) {
	walletDbPath, err := CopyEncryptedWalletDBFile(t)
	require.NoError(t, err)

	w, err := LoadExistingWallet(WalletConfig{DbPath: walletDbPath, WalletPass: walletPass})
	require.NoError(t, err)
	t.Cleanup(func() {
		w.Shutdown()
	})

	verifyTestWallet(t, w)
}

func TestLoadingEncryptedWalletWrongPassphrase(t *testing.T) {
	walletDbPath, err := CopyEncryptedWalletDBFile(t)
	require.NoError(t, err)

	w, err := LoadExistingWallet(WalletConfig{DbPath: walletDbPath, WalletPass: "wrong passphrase"})
	require.ErrorIs(t, err, ErrInvalidPassword)
	require.Nil(t, w)
}

func TestLoadingEncryptedWalletWithoutPassphrase(t *testing.T) {
	walletDbPath, err := CopyEncryptedWalletDBFile(t)
	require.NoError(t, err)

	w, err := LoadExistingWallet(WalletConfig{DbPath: walletDbPath})
	require.ErrorIs(t, err, ErrInvalidPassword)
	require.Nil(t, w)
}
