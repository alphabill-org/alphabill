package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

type mockWalletService struct {
	bills       []*Bill
	proof       *BlockProof
	trackedKeys [][]byte
}

func (m *mockWalletService) GetBills(pubKey []byte) ([]*Bill, error) {
	return m.bills, nil
}

func (m *mockWalletService) GetBlockProof(unitId []byte) (*BlockProof, error) {
	return m.proof, nil
}

func (m *mockWalletService) AddKey(pubkey []byte) error {
	for _, key := range m.trackedKeys {
		if bytes.Equal(key, pubkey) {
			return ErrKeyAlreadyExists
		}
	}
	m.trackedKeys = append(m.trackedKeys, pubkey)
	return nil
}

func TestListBillsRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{
		bills: []*Bill{{
			Id:    newUnitId(1),
			Value: 1,
		}},
	}
	startServer(t, mockService)

	res := &ListBillsResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.Len(t, res.Bills, 1)
	require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000001", res.Bills[0].Id)
	require.EqualValues(t, 1, res.Bills[0].Value)
}

func TestListBillsRequest_NilPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := doGet(t, "http://localhost:7777/api/v1/list-bills", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestListBillsRequest_InvalidPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
}

func TestListBillsRequest_SortedByOrderNumber(t *testing.T) {
	mockService := &mockWalletService{
		bills: []*Bill{
			{
				Id:          newUnitId(2),
				Value:       2,
				OrderNumber: 2,
			},
			{
				Id:          newUnitId(1),
				Value:       1,
				OrderNumber: 1,
			},
		},
	}
	startServer(t, mockService)

	res := &ListBillsResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, 2, res.Total)
	require.Len(t, res.Bills, 2)
	require.EqualValues(t, res.Bills[0].Value, 1)
	require.EqualValues(t, res.Bills[1].Value, 2)
}

func TestListBillsRequest_Paging(t *testing.T) {
	// given set of bills
	var bills []*Bill
	for i := uint64(0); i < 200; i++ {
		bills = append(bills, &Bill{
			Id:          newUnitId(i),
			Value:       i,
			OrderNumber: i,
		})
	}
	mockService := &mockWalletService{bills: bills}
	startServer(t, mockService)

	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"

	// verify by default first 100 elements are returned
	res := &ListBillsResponse{}
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 0)
	require.EqualValues(t, res.Bills[99].Value, 99)

	// verify offset=100 returns next 100 elements
	res = &ListBillsResponse{}
	httpRes = doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=100", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 100)
	require.EqualValues(t, res.Bills[99].Value, 199)

	// verify limit limits result size
	res = &ListBillsResponse{}
	httpRes = doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=100&limit=50", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 50)
	require.EqualValues(t, res.Bills[0].Value, 100)
	require.EqualValues(t, res.Bills[49].Value, 149)

	// verify out of bounds offset returns nothing
	res = &ListBillsResponse{}
	httpRes = doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=200", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 0)

	// verify limit gets capped to 100
	res = &ListBillsResponse{}
	httpRes = doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=0&limit=200", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 0)
	require.EqualValues(t, res.Bills[99].Value, 99)

	// verify out of bounds offset+limit return all available data
	res = &ListBillsResponse{}
	httpRes = doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=190&limit=100", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 10)
	require.EqualValues(t, res.Bills[0].Value, 190)
	require.EqualValues(t, res.Bills[9].Value, 199)
}

func TestBalanceRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{
		bills: []*Bill{{
			Id:    newUnitId(1),
			Value: 1,
		}},
	}
	startServer(t, mockService)

	res := &BalanceResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/balance?pubkey=%s", pk), res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.EqualValues(t, 1, res.Balance)
}

func TestBalanceRequest_NilPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := doGet(t, "http://localhost:7777/api/v1/balance", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestBalanceRequest_InvalidPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/balance?pubkey=%s", pk), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
}

func TestBlockProofRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{
		proof: &BlockProof{
			BillId:      newUnitId(1),
			BlockNumber: 1,
			BlockProof: &block.BlockProof{
				BlockHeaderHash: []byte{0},
				BlockTreeHashChain: &block.BlockTreeHashChain{
					Items: []*block.ChainItem{{Val: []byte{0}, Hash: []byte{0}}},
				},
			},
		},
	}
	startServer(t, mockService)

	res := &BlockProofResponse{}
	billId := "0x0000000000000000000000000000000000000000000000000000000000000001"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/block-proof?bill_id=%s", billId), res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000001", res.BillId)
	require.EqualValues(t, mockService.proof.BlockNumber, res.BlockNumber)
	require.Equal(t, mockService.proof.BlockProof, res.BlockProof)
}

func TestBlockProofRequest_MissingBillId(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := doGet(t, "http://localhost:7777/api/v1/block-proof", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "missing required bill_id query parameter", res.Message)
}

func TestBlockProofRequest_InvalidBillIdLength(t *testing.T) {
	startServer(t, &mockWalletService{})

	// verify bill id larger than 32 bytes returns error
	res := &ErrorResponse{}
	httpRes := doGet(t, "http://localhost:7777/api/v1/block-proof?bill_id=0x000000000000000000000000000000000000000000000000000000000000000001", res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id smaller than 32 bytes returns error
	res = &ErrorResponse{}
	httpRes = doGet(t, "http://localhost:7777/api/v1/block-proof?bill_id=0x01", res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id with correct length but missing prefix returns error
	res = &ErrorResponse{}
	httpRes = doGet(t, "http://localhost:7777/api/v1/block-proof?bill_id=0000000000000000000000000000000000000000000000000000000000000001", res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)
}

func TestBlockProofRequest_ProofDoesNotExist(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	billId := "0x0000000000000000000000000000000000000000000000000000000000000001"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/api/v1/block-proof?bill_id=%s", billId), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "block proof does not exist for given bill id", res.Message)
}

func TestAddKeyRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{}
	startServer(t, mockService)

	req := &AddKeyRequest{Pubkey: "0x000000000000000000000000000000000000000000000000000000000000000000"}
	res := &AddKeyResponse{}
	httpRes := doPost(t, "http://localhost:7777/api/v1/admin/add-key", req, res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.Len(t, mockService.trackedKeys, 1)
	pubkeyBytes, _ := hexutil.Decode(req.Pubkey)
	require.Equal(t, mockService.trackedKeys[0], pubkeyBytes)
}

func TestAddKeyRequest_KeyAlreadyExists(t *testing.T) {
	pubkey := "0x000000000000000000000000000000000000000000000000000000000000000000"
	pubkeyBytes, _ := hexutil.Decode(pubkey)
	mockService := &mockWalletService{}
	_ = mockService.AddKey(pubkeyBytes)
	startServer(t, mockService)

	req := &AddKeyRequest{Pubkey: pubkey}
	res := &ErrorResponse{}
	httpRes := doPost(t, "http://localhost:7777/api/v1/admin/add-key", req, res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, res.Message, "pubkey already exists")
}

func TestAddKeyRequest_InvalidKey(t *testing.T) {
	mockService := &mockWalletService{}
	startServer(t, mockService)

	req := &AddKeyRequest{Pubkey: "0x00"}
	res := &ErrorResponse{}
	httpRes := doPost(t, "http://localhost:7777/api/v1/admin/add-key", req, res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, res.Message, "pubkey hex string must be 68 characters long (with 0x prefix)")
}

func TestInvalidUrl_NotFound(t *testing.T) {
	startServer(t, &mockWalletService{})

	// verify request to to non-existent /api2 endpoint returns 404
	httpRes, err := http.Get("http://localhost:7777/api2/v1/list-bills")
	require.NoError(t, err)
	require.Equal(t, 404, httpRes.StatusCode)

	// verify request to to non-existent version endpoint returns 404
	httpRes, err = http.Get("http://localhost:7777/api/v5/list-bills")
	require.NoError(t, err)
	require.Equal(t, 404, httpRes.StatusCode)
}

func doGet(t *testing.T, url string, response interface{}) *http.Response {
	httpRes, err := http.Get(url)
	require.NoError(t, err)
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, _ := ioutil.ReadAll(httpRes.Body)
	fmt.Printf("GET %s response: %s\n", url, string(resBytes))
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(response)
	require.NoError(t, err)
	return httpRes
}

func doPost(t *testing.T, url string, req interface{}, res interface{}) *http.Response {
	reqBodyBytes, err := json.Marshal(req)
	require.NoError(t, err)
	httpRes, err := http.Post(url, "application/json", bytes.NewBuffer(reqBodyBytes))
	require.NoError(t, err)
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, _ := ioutil.ReadAll(httpRes.Body)
	fmt.Printf("POST %s response: %s\n", url, string(resBytes))
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(res)
	require.NoError(t, err)
	return httpRes
}

func startServer(t *testing.T, mockService *mockWalletService) {
	server := NewHttpServer(":7777", 100, mockService)
	err := server.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})
}
