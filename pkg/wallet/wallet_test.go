package wallet

import (
	"alphabill-wallet-sdk/internal/testutil"
	"alphabill-wallet-sdk/pkg/wallet/config"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
)

func TestWalletCanBeCreated(t *testing.T) {
	testutil.DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet(&config.WalletConfig{DbPath: os.TempDir()})
	defer deleteWallet(w)
	require.NoError(t, err)
	balance, err := w.GetBalance()
	require.EqualValues(t, 0, balance)
	require.NoError(t, err)
}

func TestWalletCanBeLoaded(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	walletDbPath := path.Join(wd, "testdata")

	w, err := LoadExistingWallet(&config.WalletConfig{DbPath: walletDbPath})
	require.NoError(t, err)
	require.NotNil(t, w)
}

func deleteWallet(w Wallet) {
	if w != nil {
		w.Shutdown()
		w.DeleteDb()
	}
}
