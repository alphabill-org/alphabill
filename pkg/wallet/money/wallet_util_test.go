package money

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	testclient "github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
	"github.com/holiman/uint256"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/stretchr/testify/require"
)

type (
	backendMockReturnConf struct {
		balance        uint64
		blockHeight    uint64
		billId         *uint256.Int
		billValue      uint64
		billTxHash     string
		proofList      []string
		customBillList string
		customPath     string
		customFullPath string
		customResponse string
	}
)

func CreateTestWallet(t *testing.T, br *backendMockReturnConf) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)

	err = CreateNewWallet(am, "")

	require.NoError(t, err)

	mockClient := clientmock.NewMockAlphabillClient(0, map[uint64]*block.Block{})
	_, serverAddr := mockBackendCalls(br)
	restClient, err := testclient.NewClient(serverAddr.Host)
	w, err := LoadExistingWallet(&WalletConfig{DbPath: dir}, am, restClient)
	w.AlphabillClient = mockClient
	return w, mockClient
}

func CreateTestWalletFromSeed(t *testing.T, br *backendMockReturnConf) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, testMnemonic)
	require.NoError(t, err)

	mockClient := &clientmock.MockAlphabillClient{}
	_, serverAddr := mockBackendCalls(br)
	restClient, err := testclient.NewClient(serverAddr.Host)
	w, err := LoadExistingWallet(&WalletConfig{DbPath: dir}, am, restClient)
	w.AlphabillClient = mockClient
	return w, mockClient
}

func DeleteWalletDbs(walletDir string) error {
	if err := os.Remove(filepath.Join(walletDir, account.AccountFileName)); err != nil {
		return err
	}
	return os.Remove(filepath.Join(walletDir, WalletFileName))
}

func CopyWalletDBFile(t *testing.T) (string, error) {
	_ = DeleteWalletDbs(os.TempDir())
	t.Cleanup(func() {
		_ = DeleteWalletDbs(os.TempDir())
	})
	wd, _ := os.Getwd()
	srcDir := filepath.Join(wd, "testdata", "wallet")
	return copyWalletDB(srcDir)
}

func copyWalletDB(srcDir string) (string, error) {
	dstDir := os.TempDir()
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

func mockBackendCalls(br *backendMockReturnConf) (*httptest.Server, *url.URL) {
	proofCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == br.customPath || r.URL.RequestURI() == br.customFullPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(br.customResponse))
		} else {
			switch r.URL.Path {
			case "/" + testclient.BalancePath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"balance": "%d"}`, br.balance)))
			case "/" + testclient.BlockHeightPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"blockHeight": "%d"}`, br.blockHeight)))
			case "/" + testclient.ProofPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(br.proofList[proofCount]))
				proofCount++
			case "/" + testclient.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.customBillList != "" {
					w.Write([]byte(br.customBillList))
				} else {
					w.Write([]byte(fmt.Sprintf(`{"total": 1, "bills": [{"id":"%s","value":"%d","txHash":"%s","isDcBill":false}]}`, toBillId(br.billId), br.billValue, br.billTxHash)))
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func toBillId(i *uint256.Int) string {
	return base64.StdEncoding.EncodeToString(util.Uint256ToBytes(i))
}

func createBlockProofJsonResponse(t *testing.T, bills []*Bill, overrideNonce []byte, blockNumber, timeout uint64) []string {
	var jsonList []string
	for _, b := range bills {
		w, mockClient := CreateTestWallet(t, nil)
		k, _ := w.am.GetAccountKey(0)
		var dcTx *txsystem.Transaction
		if overrideNonce != nil {
			dcTx, _ = createDustTx(k, b, overrideNonce, timeout)
		} else {
			dcTx, _ = createDustTx(k, b, calculateDcNonce([]*Bill{b}), timeout)
		}
		mockClient.SetBlock(&block.Block{
			SystemIdentifier:   alphabillMoneySystemId,
			BlockNumber:        timeout,
			PreviousBlockHash:  hash.Sum256([]byte{}),
			Transactions:       []*txsystem.Transaction{dcTx},
			UnicityCertificate: &certificates.UnicityCertificate{},
		})
		tp := &block.TxProof{
			BlockNumber: blockNumber,
			Tx:          dcTx,
			Proof: &block.BlockProof{
				BlockHeaderHash: []byte{0},
				BlockTreeHashChain: &block.BlockTreeHashChain{
					Items: []*block.ChainItem{{Val: []byte{0}, Hash: []byte{0}}},
				},
			},
		}
		b := &block.Bill{Id: util.Uint256ToBytes(b.Id), Value: b.Value, IsDcBill: b.IsDcBill, TxProof: tp, TxHash: b.TxHash}
		bills := &block.Bills{Bills: []*block.Bill{b}}
		res, _ := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(bills)
		jsonList = append(jsonList, string(res))
	}
	return jsonList
}

func createBillListJsonResponse(bills []*Bill) string {
	billVMs := make([]*money.ListBillVM, len(bills))
	for i, b := range bills {
		billVMs[i] = &money.ListBillVM{
			Id:       util.Uint256ToBytes(b.Id),
			Value:    strconv.FormatUint(b.Value, 10),
			TxHash:   b.TxHash,
			IsDCBill: b.IsDcBill,
		}
	}
	billsResponse := &money.ListBillsResponse{Bills: billVMs, Total: len(bills)}
	res, _ := json.Marshal(billsResponse)
	return string(res)
}
