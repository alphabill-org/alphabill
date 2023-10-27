package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
)

const (
	defaultPagingLimit = 100
	clientUserAgent    = "AB Explorer Backend API Client/0.1"
)

type (
	MoneyBackendClient struct {
		BaseUrl     *url.URL
		HttpClient  http.Client
		pagingLimit int

		roundNumberURL *url.URL
		unitsURL       *url.URL
		txHistoryURL   *url.URL
		infoURL        *url.URL
	}
)

const (
	TxHistoryPath   = "api/v1/tx-history"
	UnitsPath       = "api/v1/units"
	RoundNumberPath = "api/v1/round-number"
	InfoPath        = "api/v1/info"

	paramPubKey         = "pubkey"
	paramIncludeDCBills = "includeDcBills"
)

func New(baseUrl string) (*MoneyBackendClient, error) {
	if !strings.HasPrefix(baseUrl, "http://") && !strings.HasPrefix(baseUrl, "https://") {
		baseUrl = "http://" + baseUrl
	}
	u, err := url.Parse(baseUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing Money Backend Client base URL (%s): %w", baseUrl, err)
	}
	return &MoneyBackendClient{
		BaseUrl:        u,
		HttpClient:     http.Client{Timeout: time.Minute},
		roundNumberURL: u.JoinPath(RoundNumberPath),
		unitsURL:       u.JoinPath(UnitsPath),
		txHistoryURL:   u.JoinPath(TxHistoryPath),
		infoURL:        u.JoinPath(InfoPath),
		pagingLimit:    defaultPagingLimit,
	}, nil
}

func (c *MoneyBackendClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	var res *backend.RoundNumberResponse
	_, err := c.get(ctx, c.roundNumberURL, &res, false)
	if err != nil {
		return 0, fmt.Errorf("get round number request failed: %w", err)
	}
	return res.RoundNumber, nil
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
