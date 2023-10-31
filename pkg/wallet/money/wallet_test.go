package money

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	beclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/testutil"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

const (
	testMnemonic   = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"
	testPubKey0Hex = "03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	testPubKey1Hex = "02d36c574db299904b285aaeb57eb7b1fa145c43af90bec3c635c4174c224587b6"
	testPubKey2Hex = "02f6cbeacfd97ebc9b657081eb8b6c9ed3a588646d618ddbd03e198290af94c9d2"
)

func TestExistingWalletCanBeLoaded(t *testing.T) {
	homedir := t.TempDir()
	am, err := account.NewManager(homedir, "", true)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	restClient, err := beclient.New(server.URL)
	require.NoError(t, err)
	unitLocker, err := unitlock.NewUnitLocker(homedir)
	require.NoError(t, err)
	_, err = LoadExistingWallet(am, unitLocker, restClient, logger.New(t))
	require.NoError(t, err)
}

func TestWallet_GetPublicKey(t *testing.T) {
	w := CreateTestWalletFromSeed(t, nil)
	pubKey, err := w.am.GetPublicKey(0)
	require.NoError(t, err)
	require.EqualValues(t, "0x"+testPubKey0Hex, hexutil.Encode(pubKey))
}

func TestWallet_GetPublicKeys(t *testing.T) {
	w := CreateTestWalletFromSeed(t, nil)
	_, _, _ = w.am.AddAccount()

	pubKeys, err := w.am.GetPublicKeys()
	require.NoError(t, err)
	require.Len(t, pubKeys, 2)
	require.EqualValues(t, "0x"+testPubKey0Hex, hexutil.Encode(pubKeys[0]))
	require.EqualValues(t, "0x"+testPubKey1Hex, hexutil.Encode(pubKeys[1]))
}

func TestWallet_AddKey(t *testing.T) {
	w := CreateTestWalletFromSeed(t, nil)

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
	w := CreateTestWalletFromSeed(t, &testutil.BackendMockReturnConf{Balance: 10})
	balance, err := w.GetBalance(context.Background(), GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, 10, balance)
}

func TestWallet_GetBalances(t *testing.T) {
	w := CreateTestWalletFromSeed(t, &testutil.BackendMockReturnConf{Balance: 10})
	_, _, err := w.am.AddAccount()
	require.NoError(t, err)

	balances, sum, err := w.GetBalances(context.Background(), GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, 10, balances[0])
	require.EqualValues(t, 10, balances[1])
	require.EqualValues(t, 20, sum)
}

func CreateTestWalletFromSeed(t *testing.T, br *testutil.BackendMockReturnConf) *Wallet {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, testMnemonic)
	require.NoError(t, err)

	_, serverAddr := MockBackendCalls(br)
	restClient, err := beclient.New(serverAddr.Host)
	require.NoError(t, err)

	unitLocker, err := unitlock.NewUnitLocker(dir)
	require.NoError(t, err)

	w, err := LoadExistingWallet(am, unitLocker, restClient, logger.New(t))
	require.NoError(t, err)
	return w
}

func MockBackendCalls(br *testutil.BackendMockReturnConf) (*httptest.Server, *url.URL) {
	proofCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == br.CustomPath || r.URL.RequestURI() == br.CustomFullPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(br.CustomResponse))
		} else {
			path := r.URL.Path
			switch {
			case path == "/"+beclient.BalancePath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"balance": "%d"}`, br.Balance)))
			case path == "/"+beclient.RoundNumberPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"roundNumber": "%d"}`, br.RoundNumber)))
			case path == "/"+beclient.UnitsPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(br.ProofList[proofCount%len(br.ProofList)]))
				proofCount++
			case path == "/"+beclient.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.CustomBillList != "" {
					w.Write([]byte(br.CustomBillList))
				} else {
					w.Write([]byte(fmt.Sprintf(`{"bills": [{"id":"%s","value":"%d","txHash":"%s","isDcBill":false}]}`, toBase64(br.BillID), br.BillValue, br.BillTxHash)))
				}
			case strings.Contains(path, beclient.FeeCreditPath):
				w.WriteHeader(http.StatusOK)
				fcb, _ := json.Marshal(br.FeeCreditBill)
				w.Write(fcb)
			case strings.Contains(path, beclient.TransactionsPath):
				if br.PostTransactionsResponse == nil {
					w.WriteHeader(http.StatusAccepted)
				} else {
					w.WriteHeader(http.StatusInternalServerError)
					res, _ := json.Marshal(br.PostTransactionsResponse)
					w.Write(res)
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func toBase64(bytes []byte) string {
	return base64.StdEncoding.EncodeToString(bytes)
}
