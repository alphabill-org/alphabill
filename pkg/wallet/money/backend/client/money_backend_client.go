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
	ProofPath        = "api/v1/proof"
	RoundNumberPath  = "api/v1/round-number"
	FeeCreditPath    = "api/v1/fee-credit-bill"
	TransactionsPath = "api/v1/transactions"

	balanceUrlFormat     = "%v/%v?pubkey=%v&includedcbills=%v"
	listBillsUrlFormat   = "%v/%v?pubkey=%v&includedcbills=%v"
	proofUrlFormat       = "%v/%v?bill_id=%v"
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
		res = append(res, b.ToProto())
	}
	return res, nil
}

func (c *MoneyBackendClient) GetProof(billId []byte) (*wallet.Bills, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(proofUrlFormat, c.BaseUrl, ProofPath, hexutil.Encode(billId)), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get proof request: %w", err)
	}
	req.Header.Set(contentType, applicationJson)
	response, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request GetProof failed: %w", err)
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status code: %d", response.StatusCode)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GetProof response: %w", err)
	}
	var responseObject wallet.Bills
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall GetProof response data: %w", err)
	}
	return &responseObject, nil
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

func (c *MoneyBackendClient) FetchFeeCreditBill(_ context.Context, unitID []byte) (*wallet.Bill, error) {
	req, err := http.NewRequest(http.MethodGet, c.feeCreditBillURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get fee credit request: %w", err)
	}
	req.Header.Set(contentType, applicationJson)

	// set bill_id query param
	params := url.Values{}
	params.Add("bill_id", hexutil.Encode(unitID))
	req.URL.RawQuery = params.Encode()

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
	proof, err := c.GetProof(unitID)
	if err != nil {
		return nil, err
	}
	if proof == nil {
		return nil, nil
	}
	if len(proof.Bills) == 0 {
		return nil, fmt.Errorf("get proof request returned empty proof array for unit id 0x%X", unitID)
	}
	if !bytes.Equal(proof.Bills[0].TxHash, txHash) {
		// proof exists for given unitID but probably for old tx
		return nil, nil
	}
	return proof.Bills[0].TxProof, nil
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