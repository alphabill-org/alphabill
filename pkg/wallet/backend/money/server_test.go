package money

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	testhttp "github.com/alphabill-org/alphabill/internal/testutils/http"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/stretchr/testify/require"
)

const (
	pubkeyHex = "0x000000000000000000000000000000000000000000000000000000000000000000"
	billId    = "0x0000000000000000000000000000000000000000000000000000000000000001"
)

type (
	option func(service *WalletBackend) error
)

func newWalletBackend(t *testing.T, options ...option) *WalletBackend {
	service := createWalletBackend(t, &clientmock.MockAlphabillClient{})
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
				err := tx.SetBill(bill)
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

func TestListBillsRequest_Ok(t *testing.T) {
	expectedBill := &Bill{
		Id:             newUnitID(1),
		Value:          1,
		OwnerPredicate: getOwnerPredicate(pubkeyHex),
	}
	walletBackend := newWalletBackend(t, withBills(expectedBill))
	port := startServer(t, walletBackend)

	res := &ListBillsResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Len(t, res.Bills, 1)
	expectedRes := toBillVMList([]*Bill{expectedBill})
	require.Equal(t, expectedRes, res.Bills)
}

func TestListBillsRequest_NilPubKey(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills", port), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestListBillsRequest_InvalidPubKey(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s", port, pk), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
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
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, 2, res.Total)
	require.Len(t, res.Bills, 2)
	bill := res.Bills[0]
	require.EqualValues(t, bill.Value, 1)
	require.False(t, bill.IsDCBill)
	bill = res.Bills[1]
	require.EqualValues(t, res.Bills[1].Value, 2)
	require.True(t, res.Bills[1].IsDCBill)
}

func TestListBillsRequest_Paging(t *testing.T) {
	// given set of bills
	var bills []*Bill
	for i := uint64(0); i < 200; i++ {
		bills = append(bills, &Bill{
			Id:             newUnitID(i),
			Value:          i,
			OrderNumber:    i,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		})
	}
	walletService := newWalletBackend(t, withBills(bills...))
	port := startServer(t, walletService)

	// verify by default first 100 elements are returned
	res := &ListBillsResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 0)
	require.EqualValues(t, res.Bills[99].Value, 99)

	// verify offset=100 returns next 100 elements
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offset=100", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 100)
	require.EqualValues(t, res.Bills[99].Value, 199)

	// verify limit limits result size
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offset=100&limit=50", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 50)
	require.EqualValues(t, res.Bills[0].Value, 100)
	require.EqualValues(t, res.Bills[49].Value, 149)

	// verify out of bounds offset returns nothing
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offset=200", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 0)

	// verify limit gets capped to 100
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offset=0&limit=200", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 0)
	require.EqualValues(t, res.Bills[99].Value, 99)

	// verify out of bounds offset+limit return all available data
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?pubkey=%s&offset=190&limit=100", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 10)
	require.EqualValues(t, res.Bills[0].Value, 190)
	require.EqualValues(t, res.Bills[9].Value, 199)
}

func TestListBillsRequest_NoPaging(t *testing.T) {
	// given set of bills
	var bills []*Bill
	for i := uint64(0); i < 200; i++ {
		bills = append(bills, &Bill{
			Id:             newUnitID(i),
			Value:          i,
			OrderNumber:    i,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		})
	}
	walletService := newWalletBackend(t, withBills(bills...))
	port := startServer(t, walletService)

	res := &ListBillsResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/list-bills?nopaging=true&pubkey=%s", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 200)
	require.EqualValues(t, res.Bills[0].Value, 0)
	require.EqualValues(t, res.Bills[199].Value, 199)
}

func TestBalanceRequest_Ok(t *testing.T) {
	port := startServer(t, newWalletBackend(t, withBills(
		&Bill{
			Id:             newUnitID(1),
			Value:          1,
			OwnerPredicate: getOwnerPredicate(pubkeyHex),
		})))

	res := &BalanceResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/balance?pubkey=%s", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, 1, res.Balance)
}

func TestBalanceRequest_NilPubKey(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/balance", port), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestBalanceRequest_InvalidPubKey(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/balance?pubkey=%s", port, pk), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
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
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/balance?pubkey=%s", port, pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, 1, res.Balance)
}

func TestProofRequest_Ok(t *testing.T) {
	b := &Bill{
		Id:             newUnitID(1),
		Value:          1,
		TxHash:         []byte{0},
		OwnerPredicate: getOwnerPredicate(pubkeyHex),
		TxProof: &TxProof{
			BlockNumber: 1,
			Tx:          testtransaction.NewTransaction(t),
			Proof: &block.BlockProof{
				BlockHeaderHash: []byte{0},
				BlockTreeHashChain: &block.BlockTreeHashChain{
					Items: []*block.ChainItem{{Val: []byte{0}, Hash: []byte{0}}},
				},
			},
		},
	}
	walletBackend := newWalletBackend(t, withBills(b))
	port := startServer(t, walletBackend)

	response := &block.Bills{}
	httpRes := testhttp.DoGetProto(t, fmt.Sprintf("http://localhost:%d/api/v1/proof/%s?bill_id=%s", port, pubkeyHex, billId), response)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Len(t, response.Bills, 1)
	res := response.Bills[0]
	require.Equal(t, b.Id, res.Id)
	require.Equal(t, b.Value, res.Value)
	require.Equal(t, b.TxHash, res.TxHash)

	ep := b.TxProof
	ap := res.TxProof
	require.Equal(t, ep.BlockNumber, ap.BlockNumber)
	require.EqualValues(t, ep.Tx.UnitId, ap.Tx.UnitId)
	require.EqualValues(t, ep.Proof.BlockHeaderHash, ap.Proof.BlockHeaderHash)
}

func TestProofRequest_MissingBillId(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/proof/%s", port, pubkeyHex), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "missing required bill_id query parameter", res.Message)
}

func TestProofRequest_InvalidBillIdLength(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	// verify bill id larger than 32 bytes returns error
	res := &ErrorResponse{}
	billId := "0x000000000000000000000000000000000000000000000000000000000000000001"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/proof/%s?bill_id=%s", port, pubkeyHex, billId), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id smaller than 32 bytes returns error
	res = &ErrorResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/proof/%s?bill_id=0x01", port, pubkeyHex), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id with correct length but missing prefix returns error
	res = &ErrorResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/proof/%s?bill_id=%s", port, pubkeyHex, billId), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)
}

func TestProofRequest_ProofDoesNotExist(t *testing.T) {
	port := startServer(t, newWalletBackend(t))

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/proof/%s?bill_id=%s", port, pubkeyHex, billId), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "bill does not exist", res.Message)
}

func TestBlockHeightRequest_Ok(t *testing.T) {
	blockNumber := uint64(100)
	service := newWalletBackend(t, withABClient(clientmock.NewMockAlphabillClient(blockNumber, nil)))
	port := startServer(t, service)

	res := &BlockHeightResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:%d/api/v1/block-height", port), res)

	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, blockNumber, res.BlockHeight)
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

func startServer(t *testing.T, service WalletBackendService) int {
	port, err := net.GetFreePort()
	require.NoError(t, err)

	server := NewHttpServer(fmt.Sprintf("localhost:%d", port), 100, service)
	err = server.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})
	return port
}
