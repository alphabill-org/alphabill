package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
)

const (
	defaultPagingLimit = 100
	clientUserAgent    = "Money Wallet Backend API Client/0.1"
)

type (
	MoneyBackendClient struct {
		BaseUrl     *url.URL
		HttpClient  http.Client
		pagingLimit int

		balanceURL       *url.URL
		roundNumberURL   *url.URL
		unitsURL         *url.URL
		txHistoryURL     *url.URL
		listBillsURL     *url.URL
		feeCreditBillURL *url.URL
		transactionsURL  *url.URL
		infoURL          *url.URL
	}

	Observability interface {
		TracerProvider() trace.TracerProvider
	}
)

const (
	BalancePath      = "api/v1/balance"
	ListBillsPath    = "api/v1/list-bills"
	TxHistoryPath    = "api/v1/tx-history"
	UnitsPath        = "api/v1/units"
	RoundNumberPath  = "api/v1/round-number"
	FeeCreditPath    = "api/v1/fee-credit-bills"
	TransactionsPath = "api/v1/transactions"
	InfoPath         = "api/v1/info"

	paramPubKey         = "pubkey"
	paramIncludeDCBills = "includeDcBills"
)

func New(baseUrl string, observe Observability) (*MoneyBackendClient, error) {
	if !strings.HasPrefix(baseUrl, "http://") && !strings.HasPrefix(baseUrl, "https://") {
		baseUrl = "http://" + baseUrl
	}
	u, err := url.Parse(baseUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing Money Backend Client base URL (%s): %w", baseUrl, err)
	}
	return &MoneyBackendClient{
		BaseUrl:          u,
		HttpClient:       http.Client{Timeout: time.Minute, Transport: otelhttp.NewTransport(http.DefaultTransport, otelhttp.WithServerName("money_backend"), otelhttp.WithTracerProvider(observe.TracerProvider()))},
		balanceURL:       u.JoinPath(BalancePath),
		roundNumberURL:   u.JoinPath(RoundNumberPath),
		unitsURL:         u.JoinPath(UnitsPath),
		txHistoryURL:     u.JoinPath(TxHistoryPath),
		listBillsURL:     u.JoinPath(ListBillsPath),
		feeCreditBillURL: u.JoinPath(FeeCreditPath),
		transactionsURL:  u.JoinPath(TransactionsPath),
		infoURL:          u.JoinPath(InfoPath),
		pagingLimit:      defaultPagingLimit,
	}, nil
}

func (c *MoneyBackendClient) GetBalance(ctx context.Context, pubKey []byte, includeDCBills bool) (uint64, error) {
	u := *c.balanceURL
	sdk.SetQueryParam(&u, paramPubKey, hexutil.Encode(pubKey))
	sdk.SetQueryParam(&u, paramIncludeDCBills, strconv.FormatBool(includeDCBills))

	var res *backend.BalanceResponse
	_, err := c.get(ctx, &u, &res, false)
	if err != nil {
		return 0, fmt.Errorf("get balance request failed: %w", err)
	}
	return res.Balance, nil
}

func (c *MoneyBackendClient) ListBills(ctx context.Context, pubKey []byte, includeDCBills bool, offsetKey string, limit int) (*backend.ListBillsResponse, error) {
	var bills []*sdk.Bill
	for {
		responseObject, nextKey, err := c.retrieveBills(ctx, pubKey, includeDCBills, offsetKey, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch bills: %w", err)
		}
		bills = append(bills, responseObject.Bills...)
		if nextKey == "" {
			break
		}
		offsetKey = nextKey
	}
	return &backend.ListBillsResponse{Bills: bills}, nil
}

func (c *MoneyBackendClient) GetBills(ctx context.Context, pubKey []byte) ([]*sdk.Bill, error) {
	res, err := c.ListBills(ctx, pubKey, false, "", c.pagingLimit)
	if err != nil {
		return nil, err
	}
	return res.Bills, nil
}

func (c *MoneyBackendClient) GetRoundNumber(ctx context.Context) (*sdk.RoundNumber, error) {
	var res *sdk.RoundNumber
	_, err := c.get(ctx, c.roundNumberURL, &res, false)
	if err != nil {
		return nil, fmt.Errorf("get round number request failed: %w", err)
	}
	return res, nil
}

func (c *MoneyBackendClient) GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*sdk.Bill, error) {
	var fcb *sdk.Bill
	addr := c.feeCreditBillURL.JoinPath(hexutil.Encode(unitID))
	_, err := c.get(ctx, addr, &fcb, false)
	if err != nil {
		if errors.Is(err, sdk.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get fee credit bill request failed: %w", err)
	}
	return fcb, nil
}

func (c *MoneyBackendClient) PostTransactions(ctx context.Context, pubKey sdk.PubKey, txs *sdk.Transactions) error {
	b, err := cbor.Marshal(txs)
	if err != nil {
		return fmt.Errorf("failed to encode transactions: %w", err)
	}
	addr := c.transactionsURL.JoinPath(hexutil.Encode(pubKey))
	var rsp map[string]string
	if err := c.post(ctx, addr, bytes.NewBuffer(b), &rsp); err != nil {
		return fmt.Errorf("failed to send transactions: %w", err)
	}
	if len(rsp) > 0 {
		msg := "failed to process some of the transactions:\n"
		for k, v := range rsp {
			msg += k + ": " + v + "\n"
		}
		return errors.New(strings.TrimSpace(msg))
	}
	return nil
}

// GetTxProof wrapper for GetProof method to satisfy txsubmitter interface, also verifies txHash
func (c *MoneyBackendClient) GetTxProof(ctx context.Context, unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
	var res *sdk.Proof
	addr := c.unitsURL.JoinPath(hexutil.Encode(unitID), "transactions", hexutil.Encode(txHash), "proof")
	_, err := c.get(ctx, addr, &res, false)
	if err != nil {
		if errors.Is(err, sdk.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get tx proof request failed: %w", err)
	}
	return res, nil
}

func (c *MoneyBackendClient) GetTxHistory(ctx context.Context, pubKey sdk.PubKey, offset string, limit int) ([]*sdk.TxHistoryRecord, string, error) {
	var res []*sdk.TxHistoryRecord
	addr := c.txHistoryURL.JoinPath(hexutil.Encode(pubKey))
	sdk.SetPaginationParams(addr, offset, limit)

	nextKey, err := c.get(ctx, addr, &res, false)
	if err != nil {
		return nil, "", fmt.Errorf("get tx history request failed: %w", err)
	}
	return res, nextKey, nil
}

func (c *MoneyBackendClient) GetInfo(ctx context.Context) (*sdk.InfoResponse, error) {
	var res *sdk.InfoResponse
	_, err := c.get(ctx, c.infoURL, &res, false)
	if err != nil {
		return nil, fmt.Errorf("get info request failed: %w", err)
	}
	return res, nil
}

func (c *MoneyBackendClient) retrieveBills(ctx context.Context, pubKey []byte, includeDCBills bool, offsetKey string, limit int) (*backend.ListBillsResponse, string, error) {
	u := *c.listBillsURL
	sdk.SetQueryParam(&u, paramPubKey, hexutil.Encode(pubKey))
	sdk.SetQueryParam(&u, paramIncludeDCBills, strconv.FormatBool(includeDCBills))
	sdk.SetPaginationParams(&u, offsetKey, limit)

	var res *backend.ListBillsResponse
	nextKey, err := c.get(ctx, &u, &res, false)
	if err != nil {
		return nil, "", fmt.Errorf("list bills request failed: %w", err)
	}
	return res, nextKey, nil
}

/*
get executes GET request to given "addr" and decodes response body into "data" (which has to be a pointer
of the data type expected in the response).
When "allowEmptyResponse" is false then response must have a non-empty body with CBOR content.

It returns value of the offset parameter from the Link header (empty string when header is not
present, ie missing header is not error).
*/
func (c *MoneyBackendClient) get(ctx context.Context, addr *url.URL, data any, allowEmptyResponse bool) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set(sdk.UserAgent, clientUserAgent)

	rsp, err := c.HttpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request to backend failed: %w", err)
	}
	if err := sdk.DecodeResponse(rsp, http.StatusOK, data, allowEmptyResponse); err != nil {
		return "", err
	}

	pm, err := sdk.ExtractOffsetMarker(rsp)
	if err != nil {
		return "", fmt.Errorf("failed to extract position marker: %w", err)
	}
	return pm, nil
}

func (c *MoneyBackendClient) post(ctx context.Context, u *url.URL, body io.Reader, rspData any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), body)
	if err != nil {
		return fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set(sdk.UserAgent, clientUserAgent)
	req.Header.Set(sdk.ContentType, sdk.ApplicationCbor)

	rsp, err := c.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request to backend failed: %w", err)
	}
	if err := sdk.DecodeResponse(rsp, http.StatusAccepted, rspData, true); err != nil {
		return err
	}
	return nil
}
