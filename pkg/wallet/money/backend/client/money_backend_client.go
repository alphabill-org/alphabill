package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
)

type (
	MoneyBackendClient struct {
		BaseUrl    string
		HttpClient http.Client

		feeCreditBillURL   *url.URL
		lockedFeeCreditURL *url.URL
		closedFeeCreditURL *url.URL
		transactionsURL    *url.URL
	}
)

const (
	BalancePath         = "api/v1/balance"
	ListBillsPath       = "api/v1/list-bills"
	TxHistoryPath       = "api/v1/tx-history"
	ProofPath           = "api/v1/units/{unitId}/transactions/{txHash}/proof"
	RoundNumberPath     = "api/v1/round-number"
	FeeCreditPath       = "api/v1/fee-credit-bills"
	LockedFeeCreditPath = "api/v1/locked-fee-credit"
	ClosedFeeCreditPath = "api/v1/closed-fee-credit"
	TransactionsPath    = "api/v1/transactions"

	balanceUrlFormat     = "%v/%v?pubkey=%v&includeDcBills=%v"
	listBillsUrlFormat   = "%v/%v?pubkey=%v&includeDcBills=%v"
	roundNumberUrlFormat = "%v/%v"

	defaultScheme   = "http://"
	contentType     = "Content-Type"
	applicationJson = "application/json"
	applicationCbor = "application/cbor"
)

func New(baseUrl string) (*MoneyBackendClient, error) {
	if !strings.HasPrefix(baseUrl, "http://") && !strings.HasPrefix(baseUrl, "https://") {
		baseUrl = defaultScheme + baseUrl
	}
	u, err := url.Parse(baseUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing Money Backend Client base URL (%s): %w", baseUrl, err)
	}
	return &MoneyBackendClient{
		BaseUrl:            u.String(),
		HttpClient:         http.Client{Timeout: time.Minute},
		feeCreditBillURL:   u.JoinPath(FeeCreditPath),
		lockedFeeCreditURL: u.JoinPath(LockedFeeCreditPath),
		closedFeeCreditURL: u.JoinPath(ClosedFeeCreditPath),
		transactionsURL:    u.JoinPath(TransactionsPath),
	}, nil
}

func (c *MoneyBackendClient) GetBalance(ctx context.Context, pubKey []byte, includeDCBills bool) (uint64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(balanceUrlFormat, c.BaseUrl, BalancePath, hexutil.Encode(pubKey), includeDCBills), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to build get balance request: %w", err)
	}
	req.Header.Set(contentType, applicationJson)
	response, err := c.HttpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request GetBalance failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected response status code: %d", response.StatusCode)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read GetBalance response: %w", err)
	}
	var responseObject backend.BalanceResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshall GetBalance response data: %w", err)
	}
	return responseObject.Balance, nil
}

func (c *MoneyBackendClient) ListBills(ctx context.Context, pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
	offset := 0
	responseObject, err := c.retrieveBills(ctx, pubKey, includeDCBills, offset)
	if err != nil {
		return nil, err
	}
	finalResponse := responseObject

	for len(finalResponse.Bills) < finalResponse.Total {
		offset += len(responseObject.Bills)
		responseObject, err = c.retrieveBills(ctx, pubKey, includeDCBills, offset)
		if err != nil {
			return nil, err
		}
		finalResponse.Bills = append(finalResponse.Bills, responseObject.Bills...)
	}
	return finalResponse, nil
}

func (c *MoneyBackendClient) GetBills(ctx context.Context, pubKey []byte) ([]*sdk.Bill, error) {
	res, err := c.ListBills(ctx, pubKey, false)
	if err != nil {
		return nil, err
	}
	return res.Bills, nil
}

func (c *MoneyBackendClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(roundNumberUrlFormat, c.BaseUrl, RoundNumberPath), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to build GetRoundNumber request: %w", err)
	}
	req.Header.Set(contentType, applicationJson)
	response, err := c.HttpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request GetRoundNumber failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected response status code: %d", response.StatusCode)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read GetRoundNumber response: %w", err)
	}
	var responseObject backend.RoundNumberResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshall GetRoundNumber response data: %w", err)
	}
	return responseObject.RoundNumber, nil
}

func (c *MoneyBackendClient) GetFeeCreditBill(ctx context.Context, unitID sdk.UnitID) (*sdk.Bill, error) {
	urlPath := c.feeCreditBillURL.JoinPath(hexutil.Encode(unitID)).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get fee credit request: %w", err)
	}
	req.Header.Set(contentType, applicationJson)

	response, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request get fee credit failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		responseStr, _ := httputil.DumpResponse(response, true)
		return nil, fmt.Errorf("unexpected response: %s", responseStr)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read get credit bill response: %w", err)
	}
	var res sdk.Bill
	err = json.Unmarshal(responseData, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall get fee credit bill response data: %w", err)
	}
	return &res, nil
}

func (c *MoneyBackendClient) GetLockedFeeCredit(ctx context.Context, systemID []byte, fcbID []byte) (*types.TransactionRecord, error) {
	urlPath := c.lockedFeeCreditURL.
		JoinPath(hexutil.Encode(systemID)).
		JoinPath(hexutil.Encode(fcbID)).
		String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get locked fee credit request: %w", err)
	}
	req.Header.Set(contentType, applicationCbor)

	response, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request get locked fee credit failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		responseStr, _ := httputil.DumpResponse(response, true)
		return nil, fmt.Errorf("unexpected response: %s", responseStr)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read get locked fee credit response: %w", err)
	}
	var res *types.TransactionRecord
	err = cbor.Unmarshal(responseData, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall get locked fee credit response data: %w", err)
	}
	return res, nil
}

func (c *MoneyBackendClient) GetClosedFeeCredit(ctx context.Context, fcbID []byte) (*types.TransactionRecord, error) {
	urlPath := c.closedFeeCreditURL.
		JoinPath(hexutil.Encode(fcbID)).
		String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get closed fee credit request: %w", err)
	}
	req.Header.Set(contentType, applicationCbor)

	response, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request get closed fee credit failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		responseStr, _ := httputil.DumpResponse(response, true)
		return nil, fmt.Errorf("unexpected response: %s", responseStr)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read get closed fee credit response: %w", err)
	}
	var res *types.TransactionRecord
	err = cbor.Unmarshal(responseData, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall get closed fee credit response data: %w", err)
	}
	return res, nil
}

func (c *MoneyBackendClient) PostTransactions(ctx context.Context, pubKey sdk.PubKey, txs *sdk.Transactions) error {
	b, err := cbor.Marshal(txs)
	if err != nil {
		return fmt.Errorf("failed to encode transactions: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.transactionsURL.JoinPath(hexutil.Encode(pubKey)).String(), bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("failed to create send transactions request: %w", err)
	}
	req.Header.Set(contentType, applicationCbor)
	res, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send transactions (technical error): %w", err)
	}
	defer res.Body.Close() // have to close request body in case of nil error

	if res.StatusCode != http.StatusAccepted {
		errorData, err := io.ReadAll(res.Body)
		if err == nil {
			return fmt.Errorf("failed to send transactions: status %s - %s", res.Status, string(errorData))
		}
		return fmt.Errorf("failed to send transactions: status %s", res.Status)
	}
	return nil
}

// GetTxProof wrapper for GetProof method to satisfy txsubmitter interface, also verifies txHash
func (c *MoneyBackendClient) GetTxProof(ctx context.Context, unitID sdk.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/units/0x%x/transactions/0x%x/proof", c.BaseUrl, unitID, txHash), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get tx proof request: %w", err)
	}
	response, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request GetTxProof failed: %w", err)
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status code: %d", response.StatusCode)
	}
	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GetTxProof response: %w", err)
	}

	var proof *sdk.Proof
	err = cbor.Unmarshal(responseData, &proof)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall GetTxProof response data: %w", err)
	}
	return proof, nil
}

func (c *MoneyBackendClient) GetTxHistory(ctx context.Context, pubKey sdk.PubKey, offset string, limit int) ([]*sdk.TxHistoryRecord, string, error) {
	addr, err := url.Parse(c.BaseUrl)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse base url: %w", err)
	}
	u := sdk.GetURL(*addr, TxHistoryPath, hexutil.Encode(pubKey))
	sdk.SetPaginationParams(u, offset, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to build get tx proof request: %w", err)
	}
	response, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request GetTxProof failed: %w", err)
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, "", nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected response status code: %d", response.StatusCode)
	}
	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read GetTxProof response: %w", err)
	}
	var result []*sdk.TxHistoryRecord
	err = cbor.Unmarshal(responseData, &result)
	if err != nil {
		return nil, "", fmt.Errorf("failed to unmarshall GetTxProof response data: %w", err)
	}

	pm, err := sdk.ExtractOffsetMarker(response)
	if err != nil {
		return nil, "", fmt.Errorf("failed to extract position marker: %w", err)
	}
	return result, pm, nil
}

func (c *MoneyBackendClient) retrieveBills(ctx context.Context, pubKey []byte, includeDCBills bool, offset int) (*backend.ListBillsResponse, error) {
	reqUrl := fmt.Sprintf(listBillsUrlFormat, c.BaseUrl, ListBillsPath, hexutil.Encode(pubKey), includeDCBills)
	if offset > 0 {
		reqUrl = fmt.Sprintf("%v&%s=%v", reqUrl, sdk.QueryParamOffsetKey, offset)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get bills request: %w", err)
	}
	req.Header.Set(contentType, applicationJson)
	response, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request ListBills failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status code: %d", response.StatusCode)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ListBills response: %w", err)
	}
	var responseObject backend.ListBillsResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall ListBills response data: %w", err)
	}
	return &responseObject, nil
}
