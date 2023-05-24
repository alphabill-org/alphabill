package money

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	abclient "github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	beclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/holiman/uint256"
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
		feeCreditBill  *bp.Bill
	}
)

func CreateTestWallet(t *testing.T, backend BackendAPI) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)

	return CreateTestWalletWithManager(t, backend, am)
}

func CreateTestWalletWithManager(t *testing.T, backend BackendAPI, am account.Manager) (*Wallet, *clientmock.MockAlphabillClient) {
	err := CreateNewWallet(am, "")
	require.NoError(t, err)

	mockClient := clientmock.NewMockAlphabillClient(clientmock.WithMaxBlockNumber(0), clientmock.WithBlocks(map[uint64]*types.Block{}))
	w, err := LoadExistingWallet(abclient.AlphabillClientConfig{}, am, backend)
	require.NoError(t, err)
	w.AlphabillClient = mockClient
	return w, mockClient
}

func withBackendMock(t *testing.T, br *backendMockReturnConf) BackendAPI {
	_, serverAddr := mockBackendCalls(br)
	restClient, err := beclient.New(serverAddr.Host)
	require.NoError(t, err)
	return restClient
}

func CreateTestWalletFromSeed(t *testing.T, br *backendMockReturnConf) (*Wallet, *clientmock.MockAlphabillClient) {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, testMnemonic)
	require.NoError(t, err)

	mockClient := &clientmock.MockAlphabillClient{}
	_, serverAddr := mockBackendCalls(br)
	restClient, err := beclient.New(serverAddr.Host)
	require.NoError(t, err)
	w, err := LoadExistingWallet(abclient.AlphabillClientConfig{}, am, restClient)
	require.NoError(t, err)
	w.AlphabillClient = mockClient
	return w, mockClient
}

func mockBackendCalls(br *backendMockReturnConf) (*httptest.Server, *url.URL) {
	proofCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == br.customPath || r.URL.RequestURI() == br.customFullPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(br.customResponse))
		} else {
			switch r.URL.Path {
			case "/" + beclient.BalancePath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"balance": "%d"}`, br.balance)))
			case "/" + beclient.RoundNumberPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"blockHeight": "%d"}`, br.blockHeight)))
			case "/" + beclient.ProofPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(br.proofList[proofCount%len(br.proofList)]))
				proofCount++
			case "/" + beclient.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.customBillList != "" {
					w.Write([]byte(br.customBillList))
				} else {
					w.Write([]byte(fmt.Sprintf(`{"total": 1, "bills": [{"id":"%s","value":"%d","txHash":"%s","isDcBill":false}]}`, toBillId(br.billId), br.billValue, br.billTxHash)))
				}
			case "/" + beclient.FeeCreditPath:
				w.WriteHeader(http.StatusOK)
				fcb, _ := json.Marshal(br.feeCreditBill)
				w.Write(fcb)
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

func createBlockProofResponse(t *testing.T, b *Bill, overrideNonce []byte, blockNumber, timeout uint64, k *account.AccountKey) *bp.Bills {
	w, mockClient := CreateTestWallet(t, nil)
	if k == nil {
		k, _ = w.am.GetAccountKey(0)
	}
	var dcTx *types.TransactionOrder
	if overrideNonce != nil {
		dcTx, _ = txbuilder.NewDustTx(k, w.SystemID(), b.ToProto(), overrideNonce, timeout)
	} else {
		dcTx, _ = txbuilder.NewDustTx(k, w.SystemID(), b.ToProto(), calculateDcNonce([]*Bill{b}), timeout)
	}
	txRecord := &types.TransactionRecord{TransactionOrder: dcTx}
	mockClient.SetBlock(&types.Block{
		Header: &types.Header{
			SystemID:          w.SystemID(),
			PreviousBlockHash: hash.Sum256([]byte{}),
		},
		Transactions:       []*types.TransactionRecord{txRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: timeout}},
	})
	txProof := &types.TxProof{
		TransactionRecord:  txRecord,
		BlockHeaderHash:    []byte{0},
		Chain:              []*types.GenericChainItem{{Hash: []byte{0}}},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: timeout}},
	}
	return &bp.Bills{Bills: []*bp.Bill{{Id: util.Uint256ToBytes(b.Id), Value: b.Value, IsDcBill: b.IsDcBill, TxProof: txProof, TxHash: b.TxHash}}}
}

func createBlockProofJsonResponse(t *testing.T, bills []*Bill, overrideNonce []byte, blockNumber, timeout uint64, k *account.AccountKey) []string {
	var jsonList []string
	for _, b := range bills {
		bills := createBlockProofResponse(t, b, overrideNonce, blockNumber, timeout, k)
		res, _ := json.Marshal(bills)
		jsonList = append(jsonList, string(res))
	}
	return jsonList
}

func createBillListResponse(bills []*Bill) *backend.ListBillsResponse {
	billVMs := make([]*backend.ListBillVM, len(bills))
	for i, b := range bills {
		billVMs[i] = &backend.ListBillVM{
			Id:       b.GetID(),
			Value:    b.Value,
			TxHash:   b.TxHash,
			IsDCBill: b.IsDcBill,
		}
	}
	return &backend.ListBillsResponse{Bills: billVMs, Total: len(bills)}
}

func createBillListJsonResponse(bills []*Bill) string {
	billsResponse := createBillListResponse(bills)
	res, _ := json.Marshal(billsResponse)
	return string(res)
}

type backendAPIMock struct {
	getBalance         func(pubKey []byte, includeDCBills bool) (uint64, error)
	listBills          func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error)
	getBills           func(pubKey []byte) ([]*bp.Bill, error)
	getProof           func(billId []byte) (*bp.Bills, error)
	getRoundNumber     func() (uint64, error)
	fetchFeeCreditBill func(ctx context.Context, unitID []byte) (*bp.Bill, error)
}

func (b *backendAPIMock) GetBills(pubKey []byte) ([]*bp.Bill, error) {
	if b.getBills != nil {
		return b.getBills(pubKey)
	}
	return nil, errors.New("getBills not implemented")
}

func (b *backendAPIMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if b.getRoundNumber != nil {
		return b.getRoundNumber()
	}
	return 0, errors.New("getRoundNumber not implemented")
}

func (b *backendAPIMock) FetchFeeCreditBill(ctx context.Context, unitID []byte) (*bp.Bill, error) {
	if b.fetchFeeCreditBill != nil {
		return b.fetchFeeCreditBill(ctx, unitID)
	}
	return nil, errors.New("fetchFeeCreditBill not implemented")
}

func (b *backendAPIMock) GetBalance(pubKey []byte, includeDCBills bool) (uint64, error) {
	if b.getBalance != nil {
		return b.getBalance(pubKey, includeDCBills)
	}
	return 0, errors.New("getBalance not implemented")
}

func (b *backendAPIMock) ListBills(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
	if b.listBills != nil {
		return b.listBills(pubKey, includeDCBills)
	}
	return nil, errors.New("listBills not implemented")
}

func (b *backendAPIMock) GetProof(billId []byte) (*bp.Bills, error) {
	if b.getProof != nil {
		return b.getProof(billId)
	}
	return nil, errors.New("getProof not implemented")
}
