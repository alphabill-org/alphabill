package wallet

import (
	"alphabill-wallet-sdk/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWalletCanBeCreated(t *testing.T) {
	testutil.DeleteWalletDb()
	w, err := CreateNewWallet()
	defer deleteWallet(w)
	require.NoError(t, err)
	balance, err := w.GetBalance()
	require.EqualValues(t, 0, balance)
	require.NoError(t, err)
}

func TestWalletCanBeLoaded(t *testing.T) {
	testutil.DeleteWalletDb()
	w, err := CreateNewWallet()
	defer deleteWallet(w)
	require.NoError(t, err)
	w.Shutdown()

	w2, err2 := LoadExistingWallet()
	defer deleteWallet(w2)
	require.NoError(t, err2)
}

func deleteWallet(w Wallet) {
	if w != nil {
		w.Shutdown()
		w.DeleteDb()
	}
}
