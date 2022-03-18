package wallet

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutil"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
)

const walletPass = "default-wallet-pass"

func TestEncryptedWalletCanBeCreated(t *testing.T) {
	tempDir := os.TempDir()
	_ = testutil.DeleteWalletDb(tempDir)

	w, err := CreateWalletFromSeed(testMnemonic, Config{DbPath: tempDir, WalletPass: walletPass})
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	verifyTestWallet(t, err, w)
}

func TestEncryptedWalletCanBeLoaded(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	walletDbPath := path.Join(wd, "testdata", "wallet", "encrypted")

	w, err := LoadExistingWallet(Config{DbPath: walletDbPath, WalletPass: walletPass})
	require.NoError(t, err)
	t.Cleanup(func() {
		w.Shutdown()
	})

	verifyTestWallet(t, err, w)
}

func TestLoadingEncryptedWalletWrongPassphrase(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	walletDbPath := path.Join(wd, "testdata", "wallet", "encrypted")

	w, err := LoadExistingWallet(Config{DbPath: walletDbPath, WalletPass: "wrong passphrase"})
	require.NoError(t, err)
	t.Cleanup(func() {
		w.Shutdown()
	})

	ac, err := w.db.GetAccountKey()
	require.Nil(t, ac)
	require.Errorf(t, err, "error decrypting data (incorrect passphrase?)")
}
