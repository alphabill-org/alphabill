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
	"github.com/holiman/uint256"
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
			Id:    uint256.NewInt(1),
			Value: 1,
		}},
	}
	startServer(t, mockService)

	res := &ListBillsResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/list-bills?pubkey=%s", pk), res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, mockService.bills, res.Bills)
}

func TestListBillsRequest_NilPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := doGet(t, "http://localhost:7777/list-bills", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestListBillsRequest_InvalidPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/list-bills?pubkey=%s", pk), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
}

func TestBalanceRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{
		bills: []*Bill{{
			Id:    uint256.NewInt(1),
			Value: 1,
		}},
	}
	startServer(t, mockService)

	res := &BalanceResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/balance?pubkey=%s", pk), res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.EqualValues(t, 1, res.Balance)
}

func TestBalanceRequest_NilPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := doGet(t, "http://localhost:7777/balance", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestBalanceRequest_InvalidPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/balance?pubkey=%s", pk), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
}

func TestBlockProofRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{
		proof: &BlockProof{
			BillId:      uint256.NewInt(0),
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
	billId := "0x1"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/block-proof?bill_id=%s", billId), res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, mockService.proof, res.BlockProof)
}

func TestBlockProofRequest_NilBillId(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := doGet(t, "http://localhost:7777/block-proof", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "missing required bill_id query parameter", res.Message)
}

func TestBlockProofRequest_InvalidBillID(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := doGet(t, "http://localhost:7777/block-proof?bill_id=1000", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "hex string without 0x prefix", res.Message)
}

func TestBlockProofRequest_LeadingZerosBillId(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := doGet(t, "http://localhost:7777/block-proof?bill_id=0x01", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "hex number with leading zero digits", res.Message)
}

func TestAddKeyRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{}
	startServer(t, mockService)

	req := &AddKeyRequest{Pubkey: "0x000000000000000000000000000000000000000000000000000000000000000000"}
	res := &AddKeyResponse{}
	httpRes := doPost(t, "http://localhost:7777/admin/add-key", req, res)

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
	httpRes := doPost(t, "http://localhost:7777/admin/add-key", req, res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, res.Message, ErrKeyAlreadyExists.Error())
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
	server := NewHttpServer(":7777", mockService)
	err := server.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})
}
