package money

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	beclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

const (
	testMnemonic   = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"
	testPubKey0Hex = "03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	testPubKey1Hex = "02d36c574db299904b285aaeb57eb7b1fa145c43af90bec3c635c4174c224587b6"
	testPubKey2Hex = "02f6cbeacfd97ebc9b657081eb8b6c9ed3a588646d618ddbd03e198290af94c9d2"
)

func TestExistingWalletCanBeLoaded(t *testing.T) {
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	restClient, err := beclient.New(server.URL)
	require.NoError(t, err)
	_, err = LoadExistingWallet(am, restClient)
	require.NoError(t, err)
}

func TestWallet_GetPublicKey(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t, nil)
	pubKey, err := w.am.GetPublicKey(0)
	require.NoError(t, err)
	require.EqualValues(t, "0x"+testPubKey0Hex, hexutil.Encode(pubKey))
}

func TestWallet_GetPublicKeys(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t, nil)
	_, _, _ = w.am.AddAccount()

	pubKeys, err := w.am.GetPublicKeys()
	require.NoError(t, err)
	require.Len(t, pubKeys, 2)
	require.EqualValues(t, "0x"+testPubKey0Hex, hexutil.Encode(pubKeys[0]))
	require.EqualValues(t, "0x"+testPubKey1Hex, hexutil.Encode(pubKeys[1]))
}

func TestWallet_AddKey(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t, nil)

	accIdx, accPubKey, err := w.am.AddAccount()
	require.NoError(t, err)
	require.EqualValues(t, 1, accIdx)
	require.EqualValues(t, "0x"+testPubKey1Hex, hexutil.Encode(accPubKey))
	accIdx, _ = w.am.GetMaxAccountIndex()
	require.EqualValues(t, 1, accIdx)

	accIdx, accPubKey, err = w.am.AddAccount()
	require.NoError(t, err)
	require.EqualValues(t, 2, accIdx)
	require.EqualValues(t, "0x"+testPubKey2Hex, hexutil.Encode(accPubKey))
	accIdx, _ = w.am.GetMaxAccountIndex()
	require.EqualValues(t, 2, accIdx)
}

func TestWallet_GetBalance(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t, &backendMockReturnConf{balance: 10})
	balance, err := w.GetBalance(GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, 10, balance)
}

func TestWallet_GetBalances(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t, &backendMockReturnConf{balance: 10})
	_, _, err := w.am.AddAccount()
	require.NoError(t, err)

	balances, sum, err := w.GetBalances(GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, 10, balances[0])
	require.EqualValues(t, 10, balances[1])
	require.EqualValues(t, 20, sum)
}

func TestWallet_GetRoundNumber(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t, &backendMockReturnConf{roundNumber: 10})
	roundNumber, err := w.GetRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 10, roundNumber)
}
