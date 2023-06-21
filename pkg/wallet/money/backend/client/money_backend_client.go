package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
)

type (
	MoneyBackendClient struct {
		BaseUrl    string
		HttpClient http.Client

		feeCreditBillURL *url.URL
		transactionsURL  *url.URL
	}
)

const (
	BalancePath      = "api/v1/balance"
	ListBillsPath    = "api/v1/list-bills"
	ProofPath        = "api/v1/units/{unitId}/transactions/{txHash}/proof"
	RoundNumberPath  = "api/v1/round-number"
	FeeCreditPath    = "api/v1/fee-credit-bills"
	TransactionsPath = "api/v1/transactions"

	balanceUrlFormat     = "%v/%v?pubkey=%v&includedcbills=%v"
	listBillsUrlFormat   = "%v/%v?pubkey=%v&includedcbills=%v"
	roundNumberUrlFormat = "%v/%v"

	defaultScheme   = "http://"
	contentType     = "Content-Type"
	applicationJson = "application/json"
	applicationCbor = "application/cbor"
)

var (
	ErrMissingFeeCreditBill = errors.New("fee credit bill does not exist")
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
		BaseUrl:          u.String(),
		HttpClient:       http.Client{Timeout: time.Minute},
		feeCreditBillURL: u.JoinPath(FeeCreditPath),
		transactionsURL:  u.JoinPath(TransactionsPath),
	}, nil
}

func (c *MoneyBackendClient) GetBalance(pubKey []byte, includeDCBills bool) (uint64, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(balanceUrlFormat, c.BaseUrl, BalancePath, hexutil.Encode(pubKey), includeDCBills), nil)
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

func (c *MoneyBackendClient) ListBills(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
	offset := 0
	responseObject, err := c.retrieveBills(pubKey, includeDCBills, offset)
	if err != nil {
		return nil, err
	}
	finalResponse := responseObject

	for len(finalResponse.Bills) < finalResponse.Total {
		offset += len(responseObject.Bills)
		responseObject, err = c.retrieveBills(pubKey, includeDCBills, offset)
		if err != nil {
			return nil, err
		}
		finalResponse.Bills = append(finalResponse.Bills, responseObject.Bills...)
	}
	return finalResponse, nil
}

func (c *MoneyBackendClient) GetBills(pubKey []byte) ([]*wallet.Bill, error) {
	bills, err := c.ListBills(pubKey, false)
	if err != nil {
		return nil, err
	}
	var res []*wallet.Bill
	for _, b := range bills.Bills {
		res = append(res, b.ToGenericBill())
	}
	return res, nil
}

func (c *MoneyBackendClient) GetRoundNumber(_ context.Context) (uint64, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(roundNumberUrlFormat, c.BaseUrl, RoundNumberPath), nil)
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

func (c *MoneyBackendClient) GetFeeCreditBill(_ context.Context, unitID wallet.UnitID) (*wallet.Bill, error) {
	urlPath := c.feeCreditBillURL.JoinPath(hexutil.Encode(unitID)).String()
	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
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
	var res wallet.Bill
	err = json.Unmarshal(responseData, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall get fee credit bill response data: %w", err)
	}
	return &res, nil
}

func (c *MoneyBackendClient) PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
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
		return fmt.Errorf("failed to send transactions: status %s", res.Status)
	}
	return nil
}

// GetTxProof wrapper for GetProof method to satisfy txsubmitter interface, also verifies txHash
func (c *MoneyBackendClient) GetTxProof(_ context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	fmt.Printf("GetTxProof: unitID: %x, txHash: %x\n", unitID, txHash)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/units/0x%x/transactions/0x%x/proof", c.BaseUrl, unitID, txHash), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get tx proof request: %w", err)
	}
	//req.Header.Set(contentType, applicationJson)
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

	var proof *wallet.Proof
	err = cbor.Unmarshal(responseData, &proof)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall GetTxProof response data: %w", err)
	}
	return proof, nil
}

func (c *MoneyBackendClient) retrieveBills(pubKey []byte, includeDCBills bool, offset int) (*backend.ListBillsResponse, error) {
	reqUrl := fmt.Sprintf(listBillsUrlFormat, c.BaseUrl, ListBillsPath, hexutil.Encode(pubKey), includeDCBills)
	if offset > 0 {
		reqUrl = fmt.Sprintf("%v&offset=%v", reqUrl, offset)
	}
	req, err := http.NewRequest(http.MethodGet, reqUrl, nil)
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
