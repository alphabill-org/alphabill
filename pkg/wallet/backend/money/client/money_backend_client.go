package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"google.golang.org/protobuf/encoding/protojson"
)

type (
	MoneyBackendClient struct {
		BaseUrl    string
		HttpClient http.Client
	}
)

const (
	BalancePath     = "api/v1/balance"
	ListBillsPath   = "api/v1/list-bills"
	ProofPath       = "api/v1/proof"
	BlockHeightPath = "api/v1/block-height"

	balanceUrlFormat     = "%v/%v?pubkey=%v&includedcbills=%v"
	listBillsUrlFormat   = "%v/%v?pubkey=%v"
	proofUrlFormat       = "%v/%v?bill_id=%v"
	blockHeightUrlFormat = "%v/%v"

	defaultScheme   = "http://"
	contentType     = "Content-Type"
	applicationJson = "application/json"
)

func NewClient(baseUrl string) (*MoneyBackendClient, error) {
	if !strings.HasPrefix(baseUrl, "http://") && !strings.HasPrefix(baseUrl, "https://") {
		baseUrl = defaultScheme + baseUrl
	}
	u, err := url.Parse(baseUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing Money Backend Client base URL (%s): %w", baseUrl, err)
	}
	return &MoneyBackendClient{u.String(), http.Client{Timeout: time.Minute}}, nil
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
	var responseObject money.BalanceResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshall GetBalance response data: %w", err)
	}
	return responseObject.Balance, nil
}

func (c *MoneyBackendClient) ListBills(pubKey []byte) (*money.ListBillsResponse, error) {
	offset := 0
	responseObject, err := c.retrieveBills(pubKey, offset)
	if err != nil {
		return nil, err
	}
	finalResponse := responseObject

	for len(finalResponse.Bills) < finalResponse.Total {
		offset += len(responseObject.Bills)
		responseObject, err = c.retrieveBills(pubKey, offset)
		if err != nil {
			return nil, err
		}
		finalResponse.Bills = append(finalResponse.Bills, responseObject.Bills...)
	}
	return finalResponse, nil
}

func (c *MoneyBackendClient) GetProof(billId []byte) (*moneytx.Bills, error) {
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
	var responseObject moneytx.Bills
	err = protojson.Unmarshal(responseData, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall GetProof response data: %w", err)
	}
	return &responseObject, nil
}

func (c *MoneyBackendClient) GetBlockHeight() (uint64, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(blockHeightUrlFormat, c.BaseUrl, BlockHeightPath), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to build get block height request: %w", err)
	}
	req.Header.Set(contentType, applicationJson)
	response, err := c.HttpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request GetBlockHeight failed: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected response status code: %d", response.StatusCode)
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read GetBlockHeight response: %w", err)
	}
	var responseObject money.BlockHeightResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshall GetBlockHeight response data: %w", err)
	}
	return responseObject.BlockHeight, nil
}

func (c *MoneyBackendClient) retrieveBills(pubKey []byte, offset int) (*money.ListBillsResponse, error) {
	reqUrl := fmt.Sprintf(listBillsUrlFormat, c.BaseUrl, ListBillsPath, hexutil.Encode(pubKey))
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
	var responseObject money.ListBillsResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshall ListBills response data: %w", err)
	}
	return &responseObject, nil
}
