package backend

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/ainvaltin/httpsrv"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"

	testhttp "github.com/alphabill-org/alphabill/internal/testutils/http"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
)

const (
	pubkeyHex = "0x000000000000000000000000000000000000000000000000000000000000000000"
	billId    = "0x0000000000000000000000000000000000000000000000000000000000000001"
)

type (
	option func(service *WalletBackend) error
)

func newWalletBackend(t *testing.T, options ...option) *WalletBackend {
	storage, err := createTestBillStore(t)
	require.NoError(t, err)

	service := &WalletBackend{store: storage, genericWallet: sdk.New().SetABClient(&clientmock.MockAlphabillClient{}).Build()}
	for _, o := range options {
		err := o(service)
		require.NoError(t, err)
	}
	return service
}

func withBills(bills ...*Bill) option {
	return func(s *WalletBackend) error {
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

func withBillProofss(bills ...*billProof) option {
	return func(s *WalletBackend) error {
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
	return func(s *WalletBackend) error {
		s.genericWallet.AlphabillClient = client
		return nil
	}
}

func withFeeCreditBills(bills ...*Bill) option {
	return func(s *WalletBackend) error {
		return s.store.WithTransaction(func(tx BillStoreTx) error {
			for _, bill := range bills {
				err := tx.SetFeeCreditBill(bill, nil)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
}

func TestListBillsRequest_Ok(t *testing.T) {
	expectedBill := &Bill{
		Id:             newUnitID(1),
		Value:          1,
		OwnerPredicate: getOwnerPredicate(pubkeyHex),
	}
	walletBackend := newWalletBackend(t, withBills(expectedBill))
	port := startServer(t, walletBackend)

	res := &ListBillsResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Len(t, res.Bills, 1)
	expectedRes := toBillVMList([]*Bill{expectedBill})
	require.Equal(t, expectedRes, res.Bills)
}

func TestListBillsRequest_NilPubKey(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &sdk.ErrorResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills", port), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Contains(t, res.Message, "must be 68 characters long (including 0x prefix), got 0 characters")
}

func TestListBillsRequest_InvalidPubKey(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &sdk.ErrorResponse{}
	pk := "0x00"
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s", port, pk), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Contains(t, res.Message, "must be 68 characters long (including 0x prefix), got 4 characters starting 0x00")
}

func TestListBillsRequest_DCBillsIncluded(t *testing.T) {
	walletBackend := newWalletBackend(t, withBills(
		&Bill{
			Id:             newUnitID(1),
			Value:          1,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		},
		&Bill{
			Id:             newUnitID(2),
			Value:          2,
			IsDCBill:       true,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		},
	))
	port := startServer(t, walletBackend)

	res := &ListBillsResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, 2, res.Total)
	require.Len(t, res.Bills, 2)
	bill := res.Bills[0]
	require.EqualValues(t, 1, bill.Value)
	require.False(t, bill.IsDCBill)
	bill = res.Bills[1]
	require.EqualValues(t, 2, bill.Value)
	require.True(t, bill.IsDCBill)
}

func TestListBillsRequest_DCBillsExcluded(t *testing.T) {
	walletBackend := newWalletBackend(t, withBills(
		&Bill{
			Id:             newUnitID(1),
			Value:          1,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		},
		&Bill{
			Id:             newUnitID(2),
			Value:          2,
			IsDCBill:       true,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		},
	))
	port := startServer(t, walletBackend)

	res := &ListBillsResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&includedcbills=false", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, 1, res.Total)
	require.Len(t, res.Bills, 1)
	bill := res.Bills[0]
	require.EqualValues(t, 1, bill.Value)
	require.False(t, bill.IsDCBill)
}

func TestListBillsRequest_ZeroValueBillsExcluded(t *testing.T) {
	walletBackend := newWalletBackend(t, withBills(
		&Bill{
			Id:             newUnitID(1),
			Value:          1,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		},
		&Bill{
			Id:             newUnitID(2),
			Value:          0,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		},
	))
	port := startServer(t, walletBackend)

	res := &ListBillsResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, 1, res.Total)
	require.Len(t, res.Bills, 1)
	bill := res.Bills[0]
	require.EqualValues(t, 1, bill.Value)
}

func TestListBillsRequest_Paging(t *testing.T) {
	// given set of bills
	var bills []*Bill
	for i := uint64(1); i <= 200; i++ {
		bills = append(bills, &Bill{
			Id:             newUnitID(i),
			Value:          i,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		})
	}
	walletService := newWalletBackend(t, withBills(bills...))
	port := startServer(t, walletService)

	// verify by default first 100 elements are returned
	res := &ListBillsResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, 1, res.Bills[0].Value)
	require.EqualValues(t, 100, res.Bills[99].Value)

	// verify offsetKey=100 returns next 100 elements
	res = &ListBillsResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offsetKey=100", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, 101, res.Bills[0].Value)
	require.EqualValues(t, 200, res.Bills[99].Value)

	// verify Link header of the response
	var linkHdrMatcher = regexp.MustCompile("<(.*)>")
	match := linkHdrMatcher.FindStringSubmatch(httpRes.Header.Get(sdk.HeaderLink))
	if len(match) != 2 {
		t.Errorf("Link header didn't result in expected match\nHeader: %s\nmatches: %v\n", httpRes.Header.Get(sdk.HeaderLink), match)
	} else {
		u, err := url.Parse(match[1])
		if err != nil {
			t.Fatal("failed to parse Link header:", err)
		}
		if s := u.Query().Get(sdk.QueryParamOffsetKey); s != strconv.Itoa(200) {
			t.Errorf("expected %v got %s", 200, s)
		}
	}

	// verify limit limits result size
	res = &ListBillsResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offsetKey=100&limit=50", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 50)
	require.EqualValues(t, 101, res.Bills[0].Value)
	require.EqualValues(t, 150, res.Bills[49].Value)

	// verify out of bounds offset returns nothing
	res = &ListBillsResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offsetKey=200", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 0)

	// verify limit gets capped to 100
	res = &ListBillsResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offsetKey=0&limit=200", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, 1, res.Bills[0].Value)
	require.EqualValues(t, 100, res.Bills[99].Value)

	// verify out of bounds offset+limit return all available data
	res = &ListBillsResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offsetKey=190&limit=100", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 10)
	require.EqualValues(t, 191, res.Bills[0].Value)
	require.EqualValues(t, 200, res.Bills[9].Value)

	// verify no Link header in the response
	if link := httpRes.Header.Get(sdk.HeaderLink); link != "" {
		t.Errorf("unexpectedly the Link header is not empty, got %q", link)
	}
}

func TestBalanceRequest_Ok(t *testing.T) {
	port := startServer(t, newWalletBackend(t, withBills(
		&Bill{
			Id:             newUnitID(1),
			Value:          1,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		})))

	res := &BalanceResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/balance?pubkey=%s", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, 1, res.Balance)
}

func TestBalanceRequest_NilPubKey(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &sdk.ErrorResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/balance", port), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Contains(t, res.Message, "must be 68 characters long (including 0x prefix), got 0 characters")
}

func TestBalanceRequest_InvalidPubKey(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &sdk.ErrorResponse{}
	pk := "0x00"
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/balance?pubkey=%s", port, pk), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Contains(t, res.Message, "must be 68 characters long (including 0x prefix), got 4 characters starting 0x00")
}

func TestBalanceRequest_DCBillNotIncluded(t *testing.T) {
	walletBackend := newWalletBackend(t, withBills(
		&Bill{
			Id:             newUnitID(1),
			Value:          1,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		},
		&Bill{
			Id:             newUnitID(2),
			Value:          2,
			IsDCBill:       true,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		}),
	)
	port := startServer(t, walletBackend)

	res := &BalanceResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/balance?pubkey=%s", port, pubkeyHex), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, 1, res.Balance)
}

func TestProofRequest_Ok(t *testing.T) {
	tr := testtransaction.NewTransactionRecord(t)
	txHash := tr.TransactionOrder.Hash(crypto.SHA256)
	b := &Bill{
		Id:             newUnitID(1),
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
	walletBackend := newWalletBackend(t, withBillProofss(&billProof{b, p}))
	port := startServer(t, walletBackend)

	response := &sdk.Proof{}
	httpRes, err := testhttp.DoGetCbor(fmt.Sprintf("http://localhost:%d/api/v1/units/%s/transactions/0x%x/proof", port, billId, b.TxHash), response)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, b.TxHash, response.TxRecord.TransactionOrder.Hash(crypto.SHA256))
	//
	require.Equal(t, p.TxProof.UnicityCertificate.GetRoundNumber(), response.TxProof.UnicityCertificate.GetRoundNumber())
	require.EqualValues(t, p.TxRecord.TransactionOrder.UnitID(), response.TxRecord.TransactionOrder.UnitID())
	require.EqualValues(t, p.TxProof.BlockHeaderHash, response.TxProof.BlockHeaderHash)
}

func TestProofRequest_InvalidBillIdLength(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	// verify bill id larger than 32 bytes returns error
	res := &sdk.ErrorResponse{}
	billId := test.RandomBytes(34)
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/units/0x%x/transactions/0x00/proof", port, billId), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id smaller than 32 bytes returns error
	res = &sdk.ErrorResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/units/0x01/transactions/0x00/proof", port), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id with correct length but missing prefix returns error
	res = &sdk.ErrorResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/units/%x/transactions/0x00/proof", port, billId), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Contains(t, res.Message, "hex string without 0x prefix")
}

func TestProofRequest_ProofDoesNotExist(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &sdk.ErrorResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/units/%s/transactions/0x00/proof", port, billId), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, httpRes.StatusCode)
	require.Contains(t, res.Message, fmt.Sprintf("no proof found for tx 0x00 (unit %s)", billId))
}

func TestBlockHeightRequest_Ok(t *testing.T) {
	roundNumber := uint64(150)
	alphabillClient := clientmock.NewMockAlphabillClient(
		clientmock.WithMaxRoundNumber(roundNumber),
	)
	service := newWalletBackend(t, withABClient(alphabillClient))
	port := startServer(t, service)

	res := &RoundNumberResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/round-number", port), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, roundNumber, res.RoundNumber)
}

func TestInvalidUrl_NotFound(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	// verify request to to non-existent /api2 endpoint returns 404
	httpRes, err := http.Get(fmt.Sprintf("http://localhost:%d/api2/v1/list-bills", port))
	require.NoError(t, err)
	require.Equal(t, 404, httpRes.StatusCode)

	// verify request to to non-existent version endpoint returns 404
	httpRes, err = http.Get(fmt.Sprintf("http://localhost:%d/api/v5/list-bills", port))
	require.NoError(t, err)
	require.Equal(t, 404, httpRes.StatusCode)
}

func TestGetFeeCreditBillRequest_Ok(t *testing.T) {
	b := &Bill{
		Id:             newUnitID(1),
		Value:          1,
		TxHash:         []byte{0},
		OwnerPredicate: getOwnerPredicate(pubkeyHex),
	}
	//p := &sdk.Proof{
	//	TxRecord: testtransaction.NewTransactionRecord(t),
	//	TxProof: &types.TxProof{
	//		BlockHeaderHash: []byte{0},
	//		Chain:           []*types.GenericChainItem{{Hash: []byte{0}}},
	//	},
	//}
	walletBackend := newWalletBackend(t, withFeeCreditBills(b))
	port := startServer(t, walletBackend)

	response := &sdk.Bill{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/fee-credit-bills/%s", port, billId), response)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, b.Id, response.Id)
	require.Equal(t, b.Value, response.Value)
	require.Equal(t, b.TxHash, response.TxHash)
	require.Equal(t, b.IsDCBill, response.IsDcBill)

	//ep := b.TxProof
	//ap := response.TxProof
	//require.Equal(t, ep.TxProof.UnicityCertificate.GetRoundNumber(), ap.TxProof.UnicityCertificate.GetRoundNumber())
	//require.EqualValues(t, ep.TxRecord.TransactionOrder.UnitID(), ap.TxRecord.TransactionOrder.UnitID())
	//require.EqualValues(t, ep.TxProof.BlockHeaderHash, ap.TxProof.BlockHeaderHash)
}

func TestGetFeeCreditBillRequest_InvalidBillIdLength(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	// verify bill id larger than 32 bytes returns error
	res := &sdk.ErrorResponse{}
	billId := "0x000000000000000000000000000000000000000000000000000000000000000001"
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/fee-credit-bills/%s", port, billId), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "bill_id hex string must be 66 characters long (with 0x prefix)", res.Message)

	// verify bill id smaller than 32 bytes returns error
	res = &sdk.ErrorResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/fee-credit-bills/0x01", port), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "bill_id hex string must be 66 characters long (with 0x prefix)", res.Message)

	// verify bill id with correct length but missing prefix returns error
	res = &sdk.ErrorResponse{}
	httpRes, err = testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/fee-credit-bills/%s", port, billId), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "bill_id hex string must be 66 characters long (with 0x prefix)", res.Message)
}

func TestGetFeeCreditBillRequest_BillDoesNotExist(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &sdk.ErrorResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://localhost:%d/api/v1/fee-credit-bills/%s", port, billId), res)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, httpRes.StatusCode)
	require.Equal(t, "fee credit bill does not exist", res.Message)
}

func TestPostTransactionsRequest_InvalidPubkey(t *testing.T) {
	walletBackend := newWalletBackend(t)
	port := startServer(t, walletBackend)

	res := &sdk.ErrorResponse{}
	httpRes, err := testhttp.DoPost(fmt.Sprintf("http://localhost:%d/api/v1/transactions/%s", port, "invalid"), nil, res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Contains(t, res.Message, "failed to parse sender pubkey")
}

func TestPostTransactionsRequest_EmptyBody(t *testing.T) {
	walletBackend := newWalletBackend(t)
	port := startServer(t, walletBackend)

	res := &sdk.ErrorResponse{}
	httpRes, err := testhttp.DoPostCBOR(fmt.Sprintf("http://localhost:%d/api/v1/transactions/%s", port, pubkeyHex), nil, res)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "request body contained no transactions to process", res.Message)
}

func TestPostTransactionsRequest_Ok(t *testing.T) {
	walletBackend := newWalletBackend(t)
	port := startServer(t, walletBackend)

	txs := &sdk.Transactions{Transactions: []*types.TransactionOrder{
		testtransaction.NewTransactionOrder(t),
		testtransaction.NewTransactionOrder(t),
		testtransaction.NewTransactionOrder(t),
	}}

	httpRes, err := testhttp.DoPostCBOR(fmt.Sprintf("http://localhost:%d/api/v1/transactions/%s", port, pubkeyHex), txs, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, httpRes.StatusCode)
}

func startServer(t *testing.T, service WalletBackendService) int {
	port, err := net.GetFreePort()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		handler := &moneyRestAPI{Service: service, ListBillsPageLimit: 100, rw: &sdk.ResponseWriter{}}
		server := http.Server{
			Addr:              fmt.Sprintf("localhost:%d", port),
			Handler:           handler.Router(),
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
			return port
		}

		select {
		case <-time.After(50 * time.Millisecond):
		case <-tout:
			t.Fatalf("http server didn't become available within timeout")
		}
	}
}
