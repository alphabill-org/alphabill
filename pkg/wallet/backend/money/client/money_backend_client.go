package client

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"google.golang.org/protobuf/types/known/anypb"
	"io/ioutil"
	"net/http"
	"time"
)

type (
	MoneyBackendClient struct {
		baseUri    string
		httpClient http.Client
	}

	BalanceResponse struct {
		Balance uint64 `json:"balance"`
	}

	ListBillsResponse struct {
		Total int           `json:"total"`
		Bills []*ListBillVM `json:"bills"`
	}

	ListBillVM struct {
		Id       string `json:"id"`
		Value    uint64 `json:"value"`
		TxHash   string `json:"txHash"`
		IsDCBill bool   `json:"isDCBill"`
	}

	BlockHeightResponse struct {
		BlockHeight uint64 `json:"blockHeight"`
	}

	ProofsResponse struct {
		Proofs []*Bill `json:"bills"`
	}

	Bill struct {
		Id       string   `json:"id"`
		Value    uint64   `json:"value"`
		TxHash   string   `json:"txHash"`
		IsDCBill bool     `json:"isDCBill"`
		TxProofs *TxProof `json:"txProof"`
	}

	TxProof struct {
		BlockNumber uint64 `json:"blockNumber"`
		Tx          *Tx    `json:"tx"`
		Proof       *Proof `json:"proof"`
	}

	Tx struct {
		SystemId              string     `json:"systemId"`
		UnitId                string     `json:"unitId"`
		Timeout               uint64     `json:"timeout"`
		TransactionAttributes *anypb.Any `json:"transactionAttributes"`
		OwnerProof            []byte     `json:"ownerProof"`
	}

	Proof struct {
		ProofType          string              `json:"proofType"`
		BlockHeaderHash    string              `json:"blockHeaderHash"`
		TransactionHash    string              `json:"transactionsHash"`
		HashValue          string              `json:"hashValue"`
		BlockTreeHashChain *BlockTreeHashChain `json:"blockTreeHashChain"`
		SecTreeHashChain   string              `json:"secTreeHashChain"`
		UnicityCertificate *UnicityCertificate `json:"unicityCertificate"`
	}

	BlockTreeHashChain struct {
		Items *[]BlockTreeHashChainItem `json:"items"`
	}

	BlockTreeHashChainItem struct {
		Val  string `json:"val"`
		Hash string `json:"hash"`
	}

	UnicityCertificate struct {
		InputRecord            *InputRecord            `json:"inputRecord"`
		UnicitySeal            *UnicitySeal            `json:"unicitySeal"`
		UnicityTreeCertificate *UnicityTreeCertificate `json:"unicityTreeCertificate"`
	}

	InputRecord struct {
		PreviousHash string `json:"previousHash"`
		Hash         string `json:"hash"`
		BlockHash    string `json:"blockHash"`
		SummaryValue string `json:"summaryValue"`
	}

	UnicitySeal struct {
		RootChainRoundNumber uint64 `json:"rootChainRoundNumber"`
		PreviousHash         string `json:"previousHash"`
		Hash                 string `json:"hash"`
		Signatures           string `json:"signatures"`
	}

	UnicityTreeCertificate struct {
		SystemIdentifier      string `json:"systemIdentifier"`
		SiblingHashes         string `json:"siblingHashes"`
		SystemDescriptionHash string `json:"systemDescriptionHash"`
	}
)

const (
	balancePath     = "api/v1/balance"
	listBillsPath   = "api/v1/list-bills"
	proofPath       = "api/v1/proof"
	blockHeightPath = "api/v1/block-height"
)

func NewClient(baseUri string) *MoneyBackendClient {
	return &MoneyBackendClient{baseUri, http.Client{Timeout: time.Minute}}
}

func (c *MoneyBackendClient) getBalance(pubKey []byte, includeDCBills bool) (uint64, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%v/%v?pubkey=%v&includedcbills=%v", c.baseUri, balancePath, hexutil.Encode(pubKey), includeDCBills), nil)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}
	var responseObject BalanceResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return 0, err
	}

	return responseObject.Balance, nil
}

func (c *MoneyBackendClient) listBills(pubKey []byte) (*ListBillsResponse, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%v/%v?pubkey=%v", c.baseUri, listBillsPath, hexutil.Encode(pubKey)), nil)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	var responseObject ListBillsResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return nil, err
	}

	return &responseObject, nil
}

func (c *MoneyBackendClient) getProof(pubKey []byte, billId []byte) (*ProofsResponse, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%v/%v/%v?bill_id=%v", c.baseUri, proofPath, hexutil.Encode(pubKey), hexutil.Encode(billId)), nil)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	var responseObject ProofsResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return nil, err
	}

	return &responseObject, nil
}

func (c *MoneyBackendClient) getBlockHeight() (uint64, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%v/%v", c.baseUri, blockHeightPath), nil)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}
	var responseObject BlockHeightResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		return 0, err
	}

	return responseObject.BlockHeight, nil
}
