package explorer

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/ainvaltin/httpsrv"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testhttp "github.com/alphabill-org/alphabill/internal/testutils/http"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
)

const (
	pubkeyHex = "0x000000000000000000000000000000000000000000000000000000000000000000"
)

var (
	billID            = money.NewBillID(nil, []byte{1})
	feeCreditRecordID = money.NewFeeCreditRecordID(nil, []byte{1})
)

func Test_getBlockByBlockNumber(t *testing.T) {
	blockNumber := test.RandomUint64()
	storage := createTestBillStore(t)

	b := &types.Block{Header: &types.Header{}, UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber}}}

	// set block
	err := storage.Do().SetBlock(b)
	require.NoError(t, err)

	service := &ExplorerBackend{store: storage, sdk: sdk.New().SetABClient(&clientmock.MockAlphabillClient{}).Build()}
	port, _ := startServer(t, service)

	res := &types.Block{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/blocks/%d", port, blockNumber), res)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, blockNumber, res.UnicityCertificate.InputRecord.RoundNumber)
}

func Test_txHistory(t *testing.T) {
	pubkey1 := sdk.PubKey(test.RandomBytes(33))
	pubkey2 := sdk.PubKey(test.RandomBytes(33))

	storage := createTestBillStore(t)
	rec := &sdk.TxHistoryRecord{
		Kind:         sdk.OUTGOING,
		State:        sdk.UNCONFIRMED,
		CounterParty: pubkey2.Hash(),
	}
	storage.Do().StoreTxHistoryRecord(pubkey1.Hash(), rec)
	service := &ExplorerBackend{store: storage, sdk: sdk.New().SetABClient(&clientmock.MockAlphabillClient{}).Build()}

	port, api := startServer(t, service)

	makeTxHistoryRequest := func() *http.Response {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://localhost:%d/api/v1/tx-history", port), nil)
		w := httptest.NewRecorder()
		api.getTxHistory(w, req)
		return w.Result()
	}
	txHistResp := makeTxHistoryRequest()
	require.Equal(t, http.StatusOK, txHistResp.StatusCode)

	buf, err := io.ReadAll(txHistResp.Body)
	require.NoError(t, err)
	var txHistory []*sdk.TxHistoryRecord
	require.NoError(t, cbor.Unmarshal(buf, &txHistory))
	require.Len(t, txHistory, 1)
	require.Equal(t, sdk.OUTGOING, txHistory[0].Kind)
	require.Equal(t, sdk.UNCONFIRMED, txHistory[0].State)
	require.EqualValues(t, pubkey2.Hash(), txHistory[0].CounterParty)
}

func Test_txHistoryByKey(t *testing.T) {
	explorerService := &explorerBackendServiceMock{
		getTxHistoryRecordsByKey: func(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error) {
			return []*sdk.TxHistoryRecord{
				{
					Kind:         sdk.OUTGOING,
					State:        sdk.UNCONFIRMED,
					CounterParty: hash,
				},
			}, nil, nil
		},
		getTxProof: func(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
			return nil, nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 0, nil
		},
	}
	port, api := startServer(t, explorerService)

	makeTxHistoryRequest := func(pubkey sdk.PubKey) *http.Response {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://localhost:%d/api/v1/tx-history/0x%x", port, pubkey), nil)
		req = mux.SetURLVars(req, map[string]string{"pubkey": sdk.EncodeHex(pubkey)})
		w := httptest.NewRecorder()
		api.getTxHistoryByKey(w, req)
		return w.Result()
	}

	pubkey := sdk.PubKey(test.RandomBytes(33))
	txHistResp := makeTxHistoryRequest(pubkey)
	require.Equal(t, http.StatusOK, txHistResp.StatusCode)

	buf, err := io.ReadAll(txHistResp.Body)
	require.NoError(t, err)
	var txHistory []*sdk.TxHistoryRecord
	require.NoError(t, cbor.Unmarshal(buf, &txHistory))
	require.Len(t, txHistory, 1)
	require.Equal(t, sdk.OUTGOING, txHistory[0].Kind)
	require.Equal(t, sdk.UNCONFIRMED, txHistory[0].State)
	require.EqualValues(t, pubkey.Hash(), txHistory[0].CounterParty)
}

func TestProofRequest_Ok(t *testing.T) {
	tr := testtransaction.NewTransactionRecord(t)
	txHash := tr.TransactionOrder.Hash(crypto.SHA256)
	b := &Bill{
		Id:             money.NewBillID(nil, []byte{1}),
		Value:          1,
		TxHash:         txHash,
		OwnerPredicate: getOwnerPredicate(pubkeyHex),
	}
	p := &sdk.Proof{
		TxRecord: tr,
		TxProof: &types.TxProof{
			BlockHeaderHash:    []byte{0},
			Chain:              []*types.GenericChainItem{{Hash: []byte{0}}},
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
		},
	}
	walletBackend := newExplorerBackend(t, withBillProofs(&billProof{b, p}))
	port, _ := startServer(t, walletBackend)

	response := &sdk.Proof{}
	httpRes, err := testhttp.DoGetCbor(fmt.Sprintf("http://localhost:%d/api/v1/units/0x%s/transactions/0x%x/proof", port, billID, b.TxHash), response)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, b.TxHash, response.TxRecord.TransactionOrder.Hash(crypto.SHA256))
	//
	require.Equal(t, p.TxProof.UnicityCertificate.GetRoundNumber(), response.TxProof.UnicityCertificate.GetRoundNumber())
	require.EqualValues(t, p.TxRecord.TransactionOrder.UnitID(), response.TxRecord.TransactionOrder.UnitID())
	require.EqualValues(t, p.TxProof.BlockHeaderHash, response.TxProof.BlockHeaderHash)
}

func TestProofRequest_InvalidBillIdLength(t *testing.T) {
	port, _ := startServer(t, newExplorerBackend(t))

	// verify bill id larger than 33 bytes returns error
	res := &sdk.ErrorResponse{}
	billID := test.RandomBytes(34)
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/units/0x%x/transactions/0x00/proof", port, billID), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id smaller than 33 bytes returns error
	res = &sdk.ErrorResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/units/0x01/transactions/0x00/proof", port), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id with correct length but missing prefix returns error
	res = &sdk.ErrorResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/units/%x/transactions/0x00/proof", port, billID), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Contains(t, res.Message, "hex string without 0x prefix")
}

func TestProofRequest_ProofDoesNotExist(t *testing.T) {
	port, _ := startServer(t, newExplorerBackend(t))

	res := &sdk.ErrorResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/units/0x%s/transactions/0x00/proof", port, billID), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, httpRes.StatusCode)
	require.Contains(t, res.Message, fmt.Sprintf("no proof found for tx 0x00 (unit 0x%s)", billID))
}

func TestRoundNumberRequest_Ok(t *testing.T) {
	roundNumber := uint64(150)
	alphabillClient := clientmock.NewMockAlphabillClient(
		clientmock.WithMaxRoundNumber(roundNumber),
	)
	service := newExplorerBackend(t, withABClient(alphabillClient))
	port, _ := startServer(t, service)

	res := &RoundNumberResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/round-number", port), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, roundNumber, res.RoundNumber)
}

func TestInvalidUrl_NotFound(t *testing.T) {
	port, _ := startServer(t, newExplorerBackend(t))

	// verify request to to non-existent /api2 endpoint returns 404
	httpRes, err := http.Get(fmt.Sprintf("http://localhost:%d/api2/v1/list-bills", port))
	require.NoError(t, err)
	require.Equal(t, 404, httpRes.StatusCode)

	// verify request to to non-existent version endpoint returns 404
	httpRes, err = http.Get(fmt.Sprintf("http://localhost:%d/api/v5/list-bills", port))
	require.NoError(t, err)
	require.Equal(t, 404, httpRes.StatusCode)
}

func TestInfoRequest_Ok(t *testing.T) {
	service := newExplorerBackend(t)
	port, _ := startServer(t, service)

	var res *sdk.InfoResponse
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/info", port), &res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, "00000000", res.SystemID)
	require.Equal(t, "explorer backend", res.Name)
}

func verifyLinkHeader(t *testing.T, httpRes *http.Response, nextKey []byte) {
	var linkHdrMatcher = regexp.MustCompile("<(.*)>")
	match := linkHdrMatcher.FindStringSubmatch(httpRes.Header.Get(sdk.HeaderLink))
	if len(match) != 2 {
		t.Errorf("Link header didn't result in expected match\nHeader: %s\nmatches: %v\n", httpRes.Header.Get(sdk.HeaderLink), match)
	} else {
		u, err := url.Parse(match[1])
		if err != nil {
			t.Fatal("failed to parse Link header:", err)
		}
		if s := u.Query().Get(sdk.QueryParamOffsetKey); s != hexutil.Encode(nextKey) {
			t.Errorf("expected %x got %s", nextKey, s)
		}
	}
}

func verifyNoLinkHeader(t *testing.T, httpRes *http.Response) {
	if link := httpRes.Header.Get(sdk.HeaderLink); link != "" {
		t.Errorf("unexpectedly the Link header is not empty, got %q", link)
	}
}

type (
	option func(service *ExplorerBackend) error
)

func newExplorerBackend(t *testing.T, options ...option) *ExplorerBackend {
	storage := createTestBillStore(t)
	service := &ExplorerBackend{store: storage, sdk: sdk.New().SetABClient(&clientmock.MockAlphabillClient{}).Build()}
	for _, o := range options {
		err := o(service)
		require.NoError(t, err)
	}
	return service
}

func withBills(bills ...*Bill) option {
	return func(s *ExplorerBackend) error {
		return s.store.WithTransaction(func(tx BillStoreTx) error {
			for _, bill := range bills {
				err := tx.SetBill(bill, nil)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
}

type billProof struct {
	bill  *Bill
	proof *sdk.Proof
}

func withBillProofs(bills ...*billProof) option {
	return func(s *ExplorerBackend) error {
		return s.store.WithTransaction(func(tx BillStoreTx) error {
			for _, bill := range bills {
				err := tx.SetBill(bill.bill, bill.proof)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
}

func withABClient(client client.ABClient) option {
	return func(s *ExplorerBackend) error {
		s.sdk.AlphabillClient = client
		return nil
	}
}

func startServer(t *testing.T, service ExplorerBackendService) (port int, api *moneyRestAPI) {
	var err error
	port, err = net.GetFreePort()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		api = &moneyRestAPI{Service: service, ListBillsPageLimit: 100, rw: &sdk.ResponseWriter{}, SystemID: moneySystemID}
		server := http.Server{
			Addr:              fmt.Sprintf("localhost:%d", port),
			Handler:           api.Router(),
			ReadTimeout:       3 * time.Second,
			ReadHeaderTimeout: time.Second,
			WriteTimeout:      5 * time.Second,
			IdleTimeout:       30 * time.Second,
		}

		err := httpsrv.Run(ctx, server, httpsrv.ShutdownTimeout(5*time.Second))
		require.ErrorIs(t, err, context.Canceled)
	}()
	// stop the server
	t.Cleanup(func() { cancel() })

	// wait until server is up
	tout := time.After(1500 * time.Millisecond)
	for {
		if _, err := http.Get(fmt.Sprintf("http://localhost:%d", port)); err != nil {
			if !errors.Is(err, syscall.ECONNREFUSED) {
				t.Fatalf("unexpected error from http server: %v", err)
			}
		} else {
			return port, api
		}

		select {
		case <-time.After(50 * time.Millisecond):
		case <-tout:
			t.Fatalf("http server didn't become available within timeout")
		}
	}
}

type explorerBackendServiceMock struct {
	getRoundNumber           func(ctx context.Context) (uint64, error)
	getTxProof               func(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error)
	getTxHistoryRecords      func(dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error)
	getTxHistoryRecordsByKey func(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error)
}

func (m *explorerBackendServiceMock) GetBlockByBlockNumber(blocknumber uint64) (*types.Block, error) {
	//TODO
	return nil, errors.New("not implemented")
}

func (m *explorerBackendServiceMock) GetBlocks(dbStartBlockNumber uint64, count int) (res []*types.Block, prevBlockNumber uint64, err error) {
	//TODO
	return nil, 0, errors.New("not implemented")
}
func (m *explorerBackendServiceMock) GetBlockExplorerByBlockNumber(dbStartBlockNumber uint64) (res *BlockExplorer, err error) {
	//TODO
	return nil, errors.New("not implemented")
}
func (m *explorerBackendServiceMock) GetBlocksExplorer(dbStartBlockNumber uint64, count int) (res []*BlockExplorer, prevBlockNumber uint64, err error) {
	//TODO
	return nil, 0, errors.New("not implemented")
}

func (m *explorerBackendServiceMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if m.getRoundNumber != nil {
		return m.getRoundNumber(ctx)
	}
	return 0, errors.New("not implemented")
}

func (m *explorerBackendServiceMock) GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
	if m.getTxProof != nil {
		return m.getTxProof(unitID, txHash)
	}
	return nil, errors.New("not implemented")
}
func (m *explorerBackendServiceMock) GetTxHistoryRecords(dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error) {
	if m.getTxHistoryRecords != nil {
		return m.getTxHistoryRecords(dbStartKey, count)
	}
	return nil, nil, errors.New("not implemented")
}

func (m *explorerBackendServiceMock) GetTxHistoryRecordsByKey(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error) {
	if m.getTxHistoryRecordsByKey != nil {
		return m.getTxHistoryRecordsByKey(hash, dbStartKey, count)
	}
	return nil, nil, errors.New("not implemented")
}
