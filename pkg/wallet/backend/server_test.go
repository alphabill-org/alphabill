package backend

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"net/http"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testhttp "github.com/alphabill-org/alphabill/internal/testutils/http"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type mockWalletService struct {
	bills       []*Bill
	proof       *Bill
	trackedKeys [][]byte

	getBillsErr      error
	getBlockProofErr error
}

func (m *mockWalletService) GetBills(pubKey []byte) ([]*Bill, error) {
	return m.bills, m.getBillsErr
}

func (m *mockWalletService) GetBill(unitId []byte) (*Bill, error) {
	return m.proof, m.getBlockProofErr
}

func (m *mockWalletService) SetBills(pubkey []byte, bills *block.Bills) error {
	domainBills := newBillsFromProto(bills)
	m.bills = append(m.bills, domainBills...)
	return nil
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
	ep := &Bill{
		Id:    newUnitId(1),
		Value: 1,
	}
	mockService := &mockWalletService{
		bills: []*Bill{ep},
	}
	startServer(t, mockService)

	res := &ListBillsResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.Len(t, res.Bills, 1)
	ap := res.Bills[0]
	require.Equal(t, ep.Id, ap.Id)
	require.Equal(t, ep.Value, ap.Value)
}

func TestListBillsRequest_NilPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, "http://localhost:7777/api/v1/list-bills", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestListBillsRequest_InvalidPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
}

func TestListBillsRequest_PubKeyNotIndexed(t *testing.T) {
	startServer(t, &mockWalletService{getBillsErr: ErrPubKeyNotIndexed})

	res := &ErrorResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.ErrorContains(t, ErrPubKeyNotIndexed, res.Message)
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
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)

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
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 0)
	require.EqualValues(t, res.Bills[99].Value, 99)

	// verify offset=100 returns next 100 elements
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=100", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 100)
	require.EqualValues(t, res.Bills[99].Value, 199)

	// verify limit limits result size
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=100&limit=50", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 50)
	require.EqualValues(t, res.Bills[0].Value, 100)
	require.EqualValues(t, res.Bills[49].Value, 149)

	// verify out of bounds offset returns nothing
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=200", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 0)

	// verify limit gets capped to 100
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=0&limit=200", pk), res)
	require.Equal(t, 200, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 0)
	require.EqualValues(t, res.Bills[99].Value, 99)

	// verify out of bounds offset+limit return all available data
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=190&limit=100", pk), res)
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
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/balance?pubkey=%s", pk), res)

	require.Equal(t, 200, httpRes.StatusCode)
	require.EqualValues(t, 1, res.Balance)
}

func TestBalanceRequest_NilPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, "http://localhost:7777/api/v1/balance", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestBalanceRequest_InvalidPubKey(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/balance?pubkey=%s", pk), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
}

func TestBalanceRequest_PubKeyNotIndexed(t *testing.T) {
	startServer(t, &mockWalletService{getBillsErr: ErrPubKeyNotIndexed})

	res := &ErrorResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/balance?pubkey=%s", pk), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.ErrorContains(t, ErrPubKeyNotIndexed, res.Message)
}

func TestBlockProofRequest_Ok(t *testing.T) {
	b := &Bill{
		Id:     newUnitId(1),
		Value:  1,
		TxHash: []byte{0},
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
	mockService := &mockWalletService{proof: b}
	startServer(t, mockService)

	response := &block.Bills{}
	billId := "0x0000000000000000000000000000000000000000000000000000000000000001"
	httpRes := testhttp.DoGetProto(t, fmt.Sprintf("http://localhost:7777/api/v1/proof?bill_id=%s", billId), response)

	require.Equal(t, 200, httpRes.StatusCode)
	require.Len(t, response.Bills, 1)
	res := response.Bills[0]
	require.Equal(t, b.Id, res.Id)
	require.Equal(t, b.Value, res.Value)
	require.Equal(t, b.TxHash, res.TxHash)

	ep := b.TxProof
	ap := res.TxProof
	require.Equal(t, ep.BlockNumber, ap.BlockNumber)
	require.Equal(t, ep.Tx, ap.Tx)
	require.Equal(t, ep.Proof, ap.Proof)
}

func TestBlockProofRequest_MissingBillId(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, "http://localhost:7777/api/v1/proof", res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "missing required bill_id query parameter", res.Message)
}

func TestBlockProofRequest_InvalidBillIdLength(t *testing.T) {
	startServer(t, &mockWalletService{})

	// verify bill id larger than 32 bytes returns error
	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, "http://localhost:7777/api/v1/proof?bill_id=0x000000000000000000000000000000000000000000000000000000000000000001", res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id smaller than 32 bytes returns error
	res = &ErrorResponse{}
	httpRes = testhttp.DoGet(t, "http://localhost:7777/api/v1/proof?bill_id=0x01", res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id with correct length but missing prefix returns error
	res = &ErrorResponse{}
	httpRes = testhttp.DoGet(t, "http://localhost:7777/api/v1/proof?bill_id=0000000000000000000000000000000000000000000000000000000000000001", res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)
}

func TestBlockProofRequest_ErrMissingBlockProof(t *testing.T) {
	startServer(t, &mockWalletService{getBlockProofErr: ErrMissingBlockProof})

	res := &ErrorResponse{}
	billId := "0x0000000000000000000000000000000000000000000000000000000000000001"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/proof?bill_id=%s", billId), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.ErrorContains(t, ErrMissingBlockProof, res.Message)
}

func TestBlockProofRequest_ProofDoesNotExist(t *testing.T) {
	startServer(t, &mockWalletService{})

	res := &ErrorResponse{}
	billId := "0x0000000000000000000000000000000000000000000000000000000000000001"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/proof?bill_id=%s", billId), res)

	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "block proof does not exist for given bill id", res.Message)
}

func TestAddBlockProofRequest_Ok(t *testing.T) {
	_ = log.InitStdoutLogger(log.INFO)
	pubkey := make([]byte, 33)
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey)),
	}))
	gtx, _ := txConverter.ConvertTx(tx)
	txHash := gtx.Hash(crypto.SHA256)
	proof, verifiers := createBlockProofForTx(t, tx)
	service := New(nil, NewInmemoryBillStore(), verifiers)
	_ = service.AddKey(pubkey)
	startServer(t, service)

	req := &block.Bills{
		Bills: []*block.Bill{
			{
				Id:     tx.UnitId,
				Value:  txValue,
				TxHash: txHash,
				TxProof: &block.TxProof{
					BlockNumber: 1,
					Tx:          tx,
					Proof:       proof,
				},
			},
		},
	}
	res := &EmptyResponse{}
	pubkeyHex := hexutil.Encode(pubkey)
	httpRes := testhttp.DoPostProto(t, "http://localhost:7777/api/v1/proof/"+pubkeyHex, req, res)
	require.Equal(t, 200, httpRes.StatusCode)

	bills, err := service.GetBills(pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	b := bills[0]
	require.Equal(t, tx.UnitId, b.Id)
	require.Equal(t, txHash, b.TxHash)
	require.EqualValues(t, txValue, b.Value)
	txProof := b.TxProof
	require.NotNil(t, txProof)
	require.EqualValues(t, 1, txProof.BlockNumber)
	require.Equal(t, tx, txProof.Tx)
	require.NotNil(t, proof, txProof.Proof)
}

func TestAddBlockProofRequest_UnindexedKey_NOK(t *testing.T) {
	_ = log.InitStdoutLogger(log.INFO)
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
	}))
	gtx, _ := txConverter.ConvertTx(tx)
	txHash := gtx.Hash(crypto.SHA256)
	proof, verifiers := createBlockProofForTx(t, tx)

	service := New(nil, NewInmemoryBillStore(), verifiers)
	startServer(t, service)

	pubkey := make([]byte, 33)
	req := &block.Bills{
		Bills: []*block.Bill{
			{
				Id:     tx.UnitId,
				Value:  txValue,
				TxHash: txHash,
				TxProof: &block.TxProof{
					BlockNumber: 1,
					Tx:          tx,
					Proof:       proof,
				},
			},
		},
	}
	res := &ErrorResponse{}
	pubkeyHex := hexutil.Encode(pubkey)
	httpRes := testhttp.DoPostProto(t, "http://localhost:7777/api/v1/proof/"+pubkeyHex, req, res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, errKeyNotIndexed.Error(), res.Message)
}

func TestAddBlockProofRequest_InvalidPredicate_NOK(t *testing.T) {
	_ = log.InitStdoutLogger(log.INFO)
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256([]byte("invalid pub key"))),
	}))
	gtx, _ := txConverter.ConvertTx(tx)
	txHash := gtx.Hash(crypto.SHA256)
	proof, verifiers := createBlockProofForTx(t, tx)

	pubkey := make([]byte, 33)
	service := New(nil, NewInmemoryBillStore(), verifiers)
	_ = service.AddKey(pubkey)
	startServer(t, service)

	req := &block.Bills{
		Bills: []*block.Bill{
			{
				Id:     tx.UnitId,
				Value:  txValue,
				TxHash: txHash,
				TxProof: &block.TxProof{
					BlockNumber: 1,
					Tx:          tx,
					Proof:       proof,
				},
			},
		},
	}
	res := &ErrorResponse{}
	pubkeyHex := hexutil.Encode(pubkey)
	httpRes := testhttp.DoPostProto(t, "http://localhost:7777/api/v1/proof/"+pubkeyHex, req, res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, "p2pkh predicate verification failed: invalid bearer predicate", res.Message)
}

func createBlockProofForTx(t *testing.T, tx *txsystem.Transaction) (*block.BlockProof, map[string]abcrypto.Verifier) {
	b := &block.Block{
		SystemIdentifier:  alphabillMoneySystemId,
		BlockNumber:       1,
		PreviousBlockHash: hash.Sum256([]byte{}),
		Transactions:      []*txsystem.Transaction{tx},
	}
	b, verifiers := testblock.CertifyBlock(t, b, txConverter)
	genericBlock, _ := b.ToGenericBlock(txConverter)
	proof, _ := block.NewPrimaryProof(genericBlock, uint256.NewInt(0).SetBytes(tx.UnitId), crypto.SHA256)
	return proof, verifiers
}

func TestAddKeyRequest_Ok(t *testing.T) {
	mockService := &mockWalletService{}
	startServer(t, mockService)

	req := &AddKeyRequest{Pubkey: "0x000000000000000000000000000000000000000000000000000000000000000000"}
	res := &EmptyResponse{}
	httpRes := testhttp.DoPost(t, "http://localhost:7777/api/v1/admin/add-key", req, res)

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
	httpRes := testhttp.DoPost(t, "http://localhost:7777/api/v1/admin/add-key", req, res)
	require.Equal(t, 400, httpRes.StatusCode)
	require.Equal(t, res.Message, "pubkey already exists")
}

func TestAddKeyRequest_InvalidKey(t *testing.T) {
	mockService := &mockWalletService{}
	startServer(t, mockService)

	req := &AddKeyRequest{Pubkey: "0x00"}
	res := &ErrorResponse{}
	httpRes := testhttp.DoPost(t, "http://localhost:7777/api/v1/admin/add-key", req, res)
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

func startServer(t *testing.T, service WalletBackendService) {
	server := NewHttpServer(":7777", 100, service)
	err := server.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})
}
