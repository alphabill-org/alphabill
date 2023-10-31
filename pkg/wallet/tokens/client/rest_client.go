package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
)

const (
	userAgentHeader = "User-Agent"
	clientUserAgent = "Token Wallet Backend API Client/0.1"

	contentTypeHeader = "Content-Type"
	applicationCbor   = "application/cbor"

	apiPathPrefix = "/api/v1"
)

type TokenBackend struct {
	addr url.URL
	hc   *http.Client
}

/*
New creates REST API client for token wallet backend. The "abAddr" is
address of the backend, Scheme and Host fields must be assigned.
*/
func New(abAddr url.URL) *TokenBackend {
	return &TokenBackend{
		addr: abAddr,
		hc:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (tb *TokenBackend) GetToken(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error) {
	var rspData backend.TokenUnit
	_, err := tb.get(ctx, tb.getURL(apiPathPrefix, "tokens", hexutil.Encode(id)), &rspData, true)
	if err != nil {
		return nil, fmt.Errorf("get token request failed: %w", err)
	}
	return &rspData, nil
}

/*
GetTokens returns tokens owned by "owner" and matching "kind" (may be Any, ie all kinds).
For batched querying "offsetKey" must be set to the value returned by previous batch, empty
string means "start from the beginning of the dataset". The "limit" parameter allows to set
the max batch size (but smaller result set might be returned even when there is more data in
the backend ie the "offsetKey" returned is not empty).

Returns:
  - tokens matching the query;
  - offset for the next batch (if empty then there is no more data to query);
  - non-nil error when something failed;
*/
func (tb *TokenBackend) GetTokens(ctx context.Context, kind backend.Kind, owner sdk.PubKey, offset string, limit int) ([]*backend.TokenUnit, string, error) {
	addr := tb.getURL(apiPathPrefix, "kinds", kind.String(), "owners", hexutil.Encode(owner), "tokens")
	sdk.SetPaginationParams(addr, offset, limit)

	rspData := make([]*backend.TokenUnit, 0)
	pm, err := tb.get(ctx, addr, &rspData, true)
	if err != nil {
		return nil, "", fmt.Errorf("get tokens request failed: %w", err)
	}
	return rspData, pm, nil
}

/*
GetTokenTypes returns token types of particular kind (may be Any, ie all kinds), the optional "creator"
parameter allows to further filter the types by it's creator public key.
The "offsetKey" and "limit" parameters are for batched / paginated query support.

Returns:
  - token types matching the query;
  - offset for the next batch (if empty then there is no more data to query);
  - non-nil error when something failed;
*/
func (tb *TokenBackend) GetTokenTypes(ctx context.Context, kind backend.Kind, creator sdk.PubKey, offset string, limit int) ([]*backend.TokenUnitType, string, error) {
	addr := tb.getURL(apiPathPrefix, "kinds", kind.String(), "types")
	if len(creator) > 0 {
		q := addr.Query()
		q.Add("creator", hexutil.Encode(creator))
		addr.RawQuery = q.Encode()
	}
	sdk.SetPaginationParams(addr, offset, limit)

	rspData := make([]*backend.TokenUnitType, 0)
	pm, err := tb.get(ctx, addr, &rspData, true)
	if err != nil {
		return nil, "", fmt.Errorf("get token types request failed: %w", err)
	}
	return rspData, pm, nil
}

func (tb *TokenBackend) GetTypeHierarchy(ctx context.Context, id backend.TokenTypeID) ([]*backend.TokenUnitType, error) {
	rspData := make([]*backend.TokenUnitType, 0)
	_, err := tb.get(ctx, tb.getURL(apiPathPrefix, "types", hexutil.Encode(id), "hierarchy"), &rspData, true)
	if err != nil {
		return nil, fmt.Errorf("get token type hierarchy request failed: %w", err)
	}
	return rspData, nil
}

func (tb *TokenBackend) GetTxProof(ctx context.Context, unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
	var proof *sdk.Proof
	addr := tb.getURL(apiPathPrefix, "units", hexutil.Encode(unitID), "transactions", hexutil.Encode(txHash), "proof")
	_, err := tb.get(ctx, addr, &proof, false)
	if err != nil {
		if errors.Is(err, sdk.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get tx proof request failed: %w", err)
	}
	return proof, nil
}

func (tb *TokenBackend) GetRoundNumber(ctx context.Context) (uint64, error) {
	var rn backend.RoundNumberResponse
	if _, err := tb.get(ctx, tb.getURL(apiPathPrefix, "round-number"), &rn, false); err != nil {
		return 0, fmt.Errorf("get round-number request failed: %w", err)
	}
	return rn.RoundNumber, nil
}

func (tb *TokenBackend) PostTransactions(ctx context.Context, pubKey sdk.PubKey, txs *sdk.Transactions) error {
	b, err := cbor.Marshal(txs)
	if err != nil {
		return fmt.Errorf("failed to encode transactions: %w", err)
	}

	var rsp map[string]string
	err = tb.post(ctx, tb.getURL(apiPathPrefix, "transactions", hexutil.Encode(pubKey)), bytes.NewBuffer(b), &rsp)
	if err != nil {
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

func (tb *TokenBackend) GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*sdk.Bill, error) {
	var fcb *sdk.Bill
	addr := tb.getURL(apiPathPrefix, "fee-credit-bills", hexutil.Encode(unitID))
	_, err := tb.get(ctx, addr, &fcb, false)
	if err != nil {
		if errors.Is(err, sdk.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get fee credit bill request failed: %w", err)
	}
	return fcb, nil
}

func (tb *TokenBackend) GetInfo(ctx context.Context) (*sdk.InfoResponse, error) {
	var res *sdk.InfoResponse
	addr := tb.getURL(apiPathPrefix, "info")
	_, err := tb.get(ctx, addr, &res, false)
	if err != nil {
		return nil, fmt.Errorf("get info request failed: %w", err)
	}
	return res, nil
}

func (tb *TokenBackend) getURL(pathElements ...string) *url.URL {
	return sdk.GetURL(tb.addr, pathElements...)
}

/*
get executes GET request to given "addr" and decodes response body into "data" (which has to be a pointer
of the data type expected in the response).
When "allowEmptyResponse" is false then response must have a non-empty body with CBOR content.

It returns value of the offset parameter from the Link header (empty string when header is not
present, ie missing header is not error).
*/
func (tb *TokenBackend) get(ctx context.Context, addr *url.URL, data any, allowEmptyResponse bool) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set("User-Agent", clientUserAgent)

	rsp, err := tb.hc.Do(req)
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

func (tb *TokenBackend) post(ctx context.Context, u *url.URL, body io.Reader, rspData any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), body)
	if err != nil {
		return fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set(userAgentHeader, clientUserAgent)
	req.Header.Set(contentTypeHeader, applicationCbor)

	rsp, err := tb.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request to backend failed: %w", err)
	}
	if err := sdk.DecodeResponse(rsp, http.StatusAccepted, rspData, true); err != nil {
		return err
	}
	return nil
}
