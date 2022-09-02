package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/proof"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type mockWalletService struct {
	bills []*bill
	proof *blockProof
}

func (m *mockWalletService) GetBills(pubKey []byte) ([]*bill, error) {
	return m.bills, nil
}

func (m *mockWalletService) GetBlockProof(unitId []byte) (*blockProof, error) {
	return m.proof, nil
}

func TestListBillsRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{
		bills: []*bill{{
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
	pk := "0x0000000000000000000000000000000000000000000000000000000000000000"
	httpRes := doGet(t, fmt.Sprintf("http://localhost:7777/list-bills?pubkey=%s", pk), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "pubkey must be 68 bytes long (with 0x prefix)", res.Message)
}

func TestBalanceRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{
		bills: []*bill{{
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
	require.Equal(t, "pubkey must be 68 bytes long (with 0x prefix)", res.Message)
}

func TestBlockProofRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{
		proof: &blockProof{
			BillId:      []byte{0},
			BlockNumber: 1,
			BlockProof: &proof.BlockProof{
				BlockHeaderHash:    []byte{0},
				MerkleProof:        &proof.BlockMerkleProof{PathItems: []*proof.MerklePathItem{{DirectionLeft: true, PathItem: []byte{0}}}},
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
		},
	}
	startServer(t, mockService)

	res := &BlockProofResponse{}
	billId := "0x000000000000000000000000000000000000000000000000000000000000000000"
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

func doGet(t *testing.T, url string, response interface{}) *http.Response {
	httpRes, err := http.Get(url)
	require.NoError(t, err)
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, _ := ioutil.ReadAll(httpRes.Body)
	log.Info("GET %s response: %s", url, string(resBytes))
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(response)
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
