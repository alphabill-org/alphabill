package client

import (
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

	"github.com/ethereum/go-ethereum/common/hexutil"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
)

type (
	MoneyBackendClient struct {
		BaseUrl    string
		HttpClient http.Client

		feeCreditBillURL string
	}
)

const (
	BalancePath     = "api/v1/balance"
	ListBillsPath   = "api/v1/list-bills"
	ProofPath       = "api/v1/proof"
	RoundNumberPath = "api/v1/round-number"
	FeeCreditPath   = "api/v1/fee-credit-bill"

	balanceUrlFormat     = "%v/%v?pubkey=%v&includedcbills=%v"
	listBillsUrlFormat   = "%v/%v?pubkey=%v&includedcbills=%v"
	proofUrlFormat       = "%v/%v?bill_id=%v"
	roundNumberUrlFormat = "%v/%v"

	defaultScheme   = "http://"
	contentType     = "Content-Type"
	applicationJson = "application/json"
)

var (
	ErrMissingFeeCreditBill = errors.New("fee credit bill does not exist")
)

func NewClient(baseUrl string) (*MoneyBackendClient, error) {
	if !strings.HasPrefix(baseUrl, "http://") && !strings.HasPrefix(baseUrl, "https://") {
		baseUrl = defaultScheme + baseUrl
	}
	u, err := url.Parse(baseUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing Money Backend Client base URL (%s): %w", baseUrl, err)
	}
	feeCreditURL := u.JoinPath(FeeCreditPath)
	return &MoneyBackendClient{
		BaseUrl:          u.String(),
		HttpClient:       http.Client{Timeout: time.Minute},
		feeCreditBillURL: feeCreditURL.String(),
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

func (c *MoneyBackendClient) GetBills(pubKey []byte) ([]*bp.Bill, error) {
	bills, err := c.ListBills(pubKey, false)
	if err != nil {
		return nil, err
	}
	var res []*bp.Bill
	for _, b := range bills.Bills {
		res = append(res, b.ToProto())
	}
	return res, nil
}

func (c *MoneyBackendClient) GetProof(billId []byte) (*bp.Bills, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(proofUrlFormat, c.BaseUrl, ProofPath, hexutil.Encode(billId)), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get proof request: %w", err)
	}
	req.Header.Set(contentType, applicationJson)
	response, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request GetProof failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("bill does not exist")
		}
		return nil, fmt.Errorf("unexpected response status code: %d", response.StatusCode)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GetProof response: %w", err)
	}
	var responseObject bp.Bills
	err = protojson.Unmarshal(responseData, &responseObject)
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

func (c *MoneyBackendClient) FetchFeeCreditBill(_ context.Context, unitID []byte) (*bp.Bill, error) {
	req, err := http.NewRequest(http.MethodGet, c.feeCreditBillURL, nil)
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
	var res bp.Bill
	err = protojson.Unmarshal(responseData, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall get fee credit bill response data: %w", err)
	}
	return &res, nil
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
