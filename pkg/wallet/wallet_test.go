package wallet

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWalletCanBeCreated(t *testing.T) {
	w, err := CreateNewWallet()
	defer cleanup(w)
	require.NoError(t, err)
	require.EqualValues(t, 0, w.GetBalance())
}

func TestWalletCanBeLoaded(t *testing.T) {
	w, err := CreateNewWallet()
	defer cleanup(w)
	w.Shutdown()
	require.NoError(t, err)

	w, err = LoadExistingWallet()
	require.NoError(t, err)
}

func cleanup(w Wallet) {
	if w != nil {
		w.Shutdown()
		_ = w.DeleteDb()
	}
}
