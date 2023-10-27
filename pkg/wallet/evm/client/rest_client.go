package client

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/shopspring/decimal"
)

var (
	// ErrNotFound is returned when backend responded with 404 status code.
	ErrNotFound = errors.New("not found")
)

const (
	userAgentHeader = "User-Agent"
	clientUserAgent = "EVM API Client/0.1"

	apiPathPrefix   = "/api/v1"
	evmApiSubPrefix = "evm"
)

type (
	EvmClient struct {
		addr url.URL
		hc   *http.Client
	}
)

/*
New creates REST API client for token wallet backend. The "abAddr" is
address of the backend, Scheme and Host fields must be assigned.
*/
func New(abAddr url.URL) *EvmClient {
	return &EvmClient{
		addr: abAddr,
		hc:   &http.Client{Timeout: 10 * time.Second},
	}
}

var alpha2Wei = decimal.NewFromFloat(10).Pow(decimal.NewFromFloat(10))

// WeiToAlpha - converts from alpha to wei, assuming 1:1 exchange 1 "alpha" is equal to "1 eth".
// 1 wei = wei * 10^10 / 10^18
func WeiToAlpha(wei *big.Int) uint64 {
	amount := decimal.RequireFromString(wei.String())
	result := amount.Div(alpha2Wei)
	f, _ := result.Float64()
	return uint64(f)
}

// GetFeeCreditBill - simulates fee credit bill on EVM
func (e *EvmClient) GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*sdk.Bill, error) {
	balanceStr, backlink, err := e.GetBalance(ctx, unitID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read blance for addr %s: %w", hexutil.Encode(unitID), err)
	}
	balanceWei, ok := new(big.Int).SetString(balanceStr, 10)
	if !ok {
		return nil, fmt.Errorf("account %s has invalid balance %v", hexutil.Encode(unitID), balanceStr)
	}
	return &sdk.Bill{
		Id:                      unitID,
		Value:                   WeiToAlpha(balanceWei),
		TxHash:                  nil,
		FeeCreditRecordBacklink: backlink,
	}, nil
}

// todo: The methods PostTransaction(), GetRoundNumber() and GetTxProof() GetInfo() do not belong here as they are common for all
// client needs a general refactoring - it should be possible to add a generic client and not have everything together

// PostTransaction post node transaction
func (e *EvmClient) PostTransaction(ctx context.Context, tx *types.TransactionOrder) error {
	b, err := cbor.Marshal(tx)
	if err != nil {
		return fmt.Errorf("failed to encode transactions: %w", err)
	}
	if err = e.post(ctx, e.getURL(apiPathPrefix, "transactions"), bytes.NewReader(b), http.StatusAccepted, nil); err != nil {
		return fmt.Errorf("transaction send failed: %w", err)
	}
	return nil
}

// GetRoundNumber returns node round number
func (e *EvmClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	var round uint64
	if err := e.get(ctx, e.getURL(apiPathPrefix, "rounds/latest"), &round, false); err != nil {
		return 0, fmt.Errorf("get round-number request failed: %w", err)
	}
	return round, nil
}

// GetTxProof - get transaction proof for tx hash. NB! node must be configured to run with indexer.
func (e *EvmClient) GetTxProof(ctx context.Context, _ types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
	proof := struct {
		_        struct{} `cbor:",toarray"`
		TxRecord *types.TransactionRecord
		TxProof  *types.TxProof
	}{}
	addr := e.getURL(apiPathPrefix, "transactions", hex.EncodeToString(txHash))
	if err := e.get(ctx, addr, &proof, false); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get tx proof request failed: %w", err)
	}
	return &sdk.Proof{
		TxRecord: proof.TxRecord,
		TxProof:  proof.TxProof,
	}, nil
}

func (e *EvmClient) GetInfo(ctx context.Context) (*sdk.InfoResponse, error) {
	var infoResponse *sdk.InfoResponse
	addr := e.getURL(apiPathPrefix, "info")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set("User-Agent", clientUserAgent)
	rsp, err := e.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to backend failed: %w", err)
	}
	if err = decodeJsonResponse(rsp, http.StatusOK, &infoResponse, false); err != nil {
		return nil, err
	}
	return infoResponse, nil
}

func decodeJsonResponse(rsp *http.Response, successStatus int, data any, allowEmptyResponse bool) error {
	defer rsp.Body.Close()
	if rsp.StatusCode == successStatus {
		err := json.NewDecoder(rsp.Body).Decode(data)
		if err != nil && (!errors.Is(err, io.EOF) || !allowEmptyResponse) {
			return fmt.Errorf("failed to decode response body: %w", err)
		}
		return nil
	}
	bodyBytes, err := io.ReadAll(rsp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", rsp.Status)
	}
	msg := fmt.Sprintf("node responded %s: %s", rsp.Status, string(bodyBytes))
	return errors.New(msg)
}

// GetBalance - reads account balance
func (e *EvmClient) GetBalance(ctx context.Context, ethAddr []byte) (string, []byte, error) {
	resp := &struct {
		_        struct{} `cbor:",toarray"`
		Balance  string
		Backlink []byte
	}{}

	addr := e.getURL(apiPathPrefix, evmApiSubPrefix, "balance", hex.EncodeToString(ethAddr))
	err := e.get(ctx, addr, &resp, false)
	if err != nil {
		return "", nil, err
	}
	return resp.Balance, resp.Backlink, nil
}

// GetTransactionCount reads account nonce
func (e *EvmClient) GetTransactionCount(ctx context.Context, ethAddr []byte) (uint64, error) {
	resp := &struct {
		_     struct{} `cbor:",toarray"`
		Nonce uint64
	}{}
	addr := e.getURL(apiPathPrefix, evmApiSubPrefix, "transactionCount", hex.EncodeToString(ethAddr))
	err := e.get(ctx, addr, &resp, false)
	if err != nil {
		return 0, err
	}
	return resp.Nonce, nil
}

// Call execute smart contract tx without storing the result in blockchain. Can be used to simulate tx or to read state.
func (e *EvmClient) Call(ctx context.Context, callAttr *CallAttributes) (*ProcessingDetails, error) {
	b, err := cbor.Marshal(callAttr)
	if err != nil {
		return nil, fmt.Errorf("failed to encode transactions: %w", err)
	}
	callEVMResponse := &struct {
		_       struct{} `cbor:",toarray"`
		Details *ProcessingDetails
	}{}
	addr := e.getURL(apiPathPrefix, evmApiSubPrefix, "call")
	if err = e.post(ctx, addr, bytes.NewReader(b), http.StatusOK, callEVMResponse); err != nil {
		return nil, fmt.Errorf("transaction send failed: %w", err)
	}
	return callEVMResponse.Details, nil
}

func (e *EvmClient) getURL(pathElements ...string) *url.URL {
	return sdk.GetURL(e.addr, pathElements...)
}

/*
get executes GET request to given "addr" and decodes response body into "data" (which has to be a pointer
of the data type expected in the response).
When "allowEmptyResponse" is false then response must have a non-empty body with CBOR content.

It returns value of the offset parameter from the Link header (empty string when header is not
present, ie missing header is not error).
*/
func (e *EvmClient) get(ctx context.Context, addr *url.URL, data any, allowEmptyResponse bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set("User-Agent", clientUserAgent)
	rsp, err := e.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request to backend failed: %w", err)
	}
	if err = decodeResponse(rsp, http.StatusOK, data, allowEmptyResponse); err != nil {
		return err
	}
	return nil
}

func (e *EvmClient) post(ctx context.Context, u *url.URL, body io.Reader, okCode int, rspData any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), body)
	if err != nil {
		return fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set(userAgentHeader, clientUserAgent)

	rsp, err := e.hc.Do(req)
	if err != nil {
		return fmt.Errorf("send evm node request failed: %w", err)
	}
	if err = decodeResponse(rsp, okCode, rspData, true); err != nil {
		return err
	}
	return nil
}

/*
When "rsp" StatusCode is equal to "successStatus" response body is decoded into "data".
In case of some other response status body is expected to contain error response json struct.
*/
func decodeResponse(rsp *http.Response, successStatus int, data any, allowEmptyResponse bool) error {
	defer func() { _ = rsp.Body.Close() }()

	if rsp.StatusCode == successStatus {
		// no response data expected
		if data == nil {
			return nil
		}
		err := cbor.NewDecoder(rsp.Body).Decode(data)
		if err != nil && (!errors.Is(err, io.EOF) || !allowEmptyResponse) {
			return fmt.Errorf("failed to decode response body: %w", err)
		}
		return nil
	}
	switch {
	case rsp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	default:
		errInfo := &struct {
			_   struct{} `cbor:",toarray"`
			Err string
		}{}
		if err := cbor.NewDecoder(rsp.Body).Decode(errInfo); err != nil {
			return fmt.Errorf("%s", rsp.Status)
		}
		return fmt.Errorf("%s, %s", rsp.Status, errInfo.Err)
	}
}
