package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
)

func Test_setPaginationParams(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pos   string // position marker aka offset key
		limit int    // batch size
		res   string // expected query string
	}{
		{pos: "", limit: 0, res: ""},
		{pos: "", limit: -1, res: ""},
		{pos: "", limit: 10, res: "?limit=10"},
		{pos: "a01b", limit: 0, res: "?offsetKey=a01b"},
		{pos: "a01b", limit: -2, res: "?offsetKey=a01b"},
		{pos: "a01b", limit: 20, res: "?limit=20&offsetKey=a01b"},
	}

	for x, tc := range cases {
		u := url.URL{}
		sdk.SetPaginationParams(&u, tc.pos, tc.limit)
		if r := u.String(); r != tc.res {
			t.Errorf("test case [%d] expected %q, got %q", x, tc.res, r)
		}
	}
}

func Test_get(t *testing.T) {
	t.Parallel()

	t.Run("http request fails", func(t *testing.T) {
		cli := &TokenBackend{
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("nope, won't work") },
			}},
		}
		pos, err := cli.get(context.Background(), &url.URL{Scheme: "http", Host: "localhost", Path: "api/v1"}, nil, true)
		require.EqualError(t, err, `request to backend failed: Get "http://localhost/api/v1": nope, won't work`)
		require.Empty(t, pos)
	})

	t.Run("valid request is built", func(t *testing.T) {
		cli := &TokenBackend{
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost:8000/api/v1/path?queryParam=foo` {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}

					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusOK)
					return w.Result(), nil
				},
			}},
		}

		var data int
		pos, err := cli.get(context.Background(), &url.URL{Scheme: "http", Host: "localhost:8000", Path: "api/v1/path", RawQuery: "queryParam=foo"}, &data, true)
		require.NoError(t, err)
		require.Empty(t, pos)
		require.Empty(t, data)
	})

	createClient := func(t *testing.T, status int, respBody string) *TokenBackend {
		t.Helper()
		return &TokenBackend{
			hc: &http.Client{
				Transport: &mockRoundTripper{
					do: func(r *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						w.WriteHeader(status)
						if _, err := w.WriteString(respBody); err != nil {
							t.Errorf("failed to write response body: %v", err)
						}
						return w.Result(), nil
					},
				},
			},
		}
	}

	t.Run("status OK, invalid body", func(t *testing.T) {
		cli := createClient(t, http.StatusOK, `"string"`)
		var data int
		pos, err := cli.get(context.Background(), &url.URL{Scheme: "http", Host: "localhost"}, &data, true)
		require.EqualError(t, err, `failed to decode response body: json: cannot unmarshal string into Go value of type int`)
		require.Empty(t, pos)
		require.Zero(t, data)
	})

	t.Run("status OK, valid body", func(t *testing.T) {
		cli := createClient(t, http.StatusOK, `42`)
		var data int
		pos, err := cli.get(context.Background(), &url.URL{Scheme: "http", Host: "localhost"}, &data, false)
		require.NoError(t, err)
		require.Empty(t, pos)
		require.Equal(t, 42, data)
	})

	t.Run("status BadRequest, invalid body", func(t *testing.T) {
		cli := createClient(t, http.StatusBadRequest, `this not error response struct`)
		var data int
		pos, err := cli.get(context.Background(), &url.URL{Scheme: "http", Host: "localhost"}, &data, true)
		require.EqualError(t, err, `failed to decode error from the response body (400 Bad Request): invalid character 'h' in literal true (expecting 'r')`)
		require.Empty(t, pos)
		require.Zero(t, data)
	})

	t.Run("status BadRequest, valid body", func(t *testing.T) {
		cli := createClient(t, http.StatusBadRequest, `{"message":"not good"}`)
		var data int
		pos, err := cli.get(context.Background(), &url.URL{Scheme: "http", Host: "localhost"}, &data, true)
		require.ErrorIs(t, err, sdk.ErrInvalidRequest)
		require.EqualError(t, err, `backend responded 400 Bad Request: not good: invalid request`)
		require.Empty(t, pos)
		require.Empty(t, data)
	})

	t.Run("status InternalServerError, valid body", func(t *testing.T) {
		cli := createClient(t, http.StatusInternalServerError, `{"message":"not good"}`)
		var data int
		pos, err := cli.get(context.Background(), &url.URL{Scheme: "http", Host: "localhost"}, &data, true)
		require.NotErrorIs(t, err, sdk.ErrInvalidRequest, "server side error shouldn't be reported as invalid request error")
		require.EqualError(t, err, `backend responded 500 Internal Server Error: not good`)
		require.Empty(t, pos)
		require.Empty(t, data)
	})

	clientWithHeader := func(t *testing.T, status int, linkHeader string) *TokenBackend {
		return &TokenBackend{
			hc: &http.Client{
				Transport: &mockRoundTripper{
					do: func(r *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						w.Header().Set("Link", linkHeader)
						w.WriteHeader(http.StatusOK)
						return w.Result(), nil
					},
				},
			},
		}
	}

	t.Run("invalid link header", func(t *testing.T) {
		cli := clientWithHeader(t, 200, "foo bar")
		var data int
		pos, err := cli.get(context.Background(), &url.URL{Scheme: "http", Host: "localhost"}, &data, true)
		require.Contains(t, err.Error(), `failed to extract position marker: link header didn't result in expected match`)
		require.Empty(t, pos)
	})

	t.Run("position marker is extracted correctly", func(t *testing.T) {
		cli := clientWithHeader(t, 200, `<http://localhost/something?offsetKey=abc&some=garbage>; rel="next"`)
		var data int
		pos, err := cli.get(context.Background(), &url.URL{Scheme: "http", Host: "localhost"}, &data, true)
		require.NoError(t, err)
		require.Equal(t, "abc", pos)
	})
}

func Test_GetRoundNumber(t *testing.T) {
	t.Parallel()

	t.Run("valid request is built", func(t *testing.T) {
		cli := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/round-number` {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}

					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusOK)
					if _, err := w.WriteString(`{"roundNumber": "0"}`); err != nil {
						t.Errorf("failed to write response body: %v", err)
					}
					return w.Result(), nil
				},
			}},
		}

		rn, err := cli.GetRoundNumber(context.Background())
		require.NoError(t, err)
		require.Zero(t, rn)
	})

	createClient := func(t *testing.T, respBody string) *TokenBackend {
		t.Helper()
		return &TokenBackend{
			hc: &http.Client{
				Transport: &mockRoundTripper{
					do: func(r *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						if _, err := w.WriteString(respBody); err != nil {
							t.Errorf("failed to write response body: %v", err)
						}
						return w.Result(), nil
					},
				},
			},
		}
	}

	t.Run("backend returns empty response body", func(t *testing.T) {
		cli := createClient(t, ``)
		rn, err := cli.GetRoundNumber(context.Background())
		require.EqualError(t, err, `get round-number request failed: failed to decode response body: EOF`)
		require.Zero(t, rn)
	})

	t.Run("invalid response: negative value", func(t *testing.T) {
		cli := createClient(t, `{"roundNumber": "-8"}`)
		rn, err := cli.GetRoundNumber(context.Background())
		require.EqualError(t, err, `get round-number request failed: failed to decode response body: json: cannot unmarshal number -8 into Go struct field RoundNumberResponse.roundNumber of type uint64`)
		require.Zero(t, rn)
	})

	t.Run("success", func(t *testing.T) {
		cli := createClient(t, `{"roundNumber": "3"}`)
		rn, err := cli.GetRoundNumber(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 3, rn)
	})
}

func Test_GetToken(t *testing.T) {
	t.Parallel()

	t.Run("valid request is built", func(t *testing.T) {
		tokenID := test.RandomBytes(32)
		cli := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					tokenIdStr := hexutil.Encode(tokenID)
					expURL := fmt.Sprintf("http://localhost/api/v1/tokens/%s", tokenIdStr)
					if r.URL.String() != expURL {
						t.Errorf("expected request URL %q, got %q", expURL, r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}

					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusOK)
					return w.Result(), nil
				},
			}},
		}

		data, err := cli.GetToken(context.Background(), tokenID)
		require.NoError(t, err)
		require.Empty(t, data)
	})

	createClient := func(t *testing.T, respBody *backend.TokenUnit) *TokenBackend {
		return &TokenBackend{
			hc: &http.Client{
				Transport: &mockRoundTripper{
					do: func(r *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						if respBody == nil {
							w.WriteHeader(http.StatusNotFound)
							w.WriteString(`{"message":"no token with this id"}`)
							return w.Result(), nil
						}

						err := json.NewEncoder(w).Encode(respBody)
						return w.Result(), err
					},
				},
			},
		}
	}

	t.Run("no token with given id", func(t *testing.T) {
		cli := createClient(t, nil)
		data, err := cli.GetToken(context.Background(), test.RandomBytes(32))
		require.EqualError(t, err, `get token request failed: no token with this id: not found`)
		require.ErrorIs(t, err, sdk.ErrNotFound)
		require.Empty(t, data)
	})

	t.Run("token found", func(t *testing.T) {
		token := &backend.TokenUnit{
			ID:     test.RandomBytes(32),
			Symbol: "OUCH!",
			Amount: 10000,
		}
		cli := createClient(t, token)
		data, err := cli.GetToken(context.Background(), token.ID)
		require.NoError(t, err)
		require.Equal(t, token, data)
	})
}

func Test_GetTokens(t *testing.T) {
	t.Parallel()

	ownerID := test.RandomBytes(33)

	t.Run("valid request is built", func(t *testing.T) {
		cli := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					pubKeyHex := hexutil.Encode(ownerID)
					expURL := fmt.Sprintf("http://localhost/api/v1/kinds/all/owners/%s/tokens?limit=10", pubKeyHex)
					if r.URL.String() != expURL {
						t.Errorf("expected request URL %q, got %q", expURL, r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}

					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusOK)
					return w.Result(), nil
				},
			}},
		}

		data, offset, err := cli.GetTokens(context.Background(), backend.Any, ownerID, "", 10)
		require.NoError(t, err)
		require.Empty(t, data)
		require.Empty(t, offset)
	})

	createClient := func(t *testing.T, respBody []*backend.TokenUnit) *TokenBackend {
		return &TokenBackend{
			hc: &http.Client{
				Transport: &mockRoundTripper{
					do: func(r *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						err := json.NewEncoder(w).Encode(respBody)
						return w.Result(), err
					},
				},
			},
		}
	}

	t.Run("no data in the response", func(t *testing.T) {
		cli := createClient(t, nil)
		data, offset, err := cli.GetTokens(context.Background(), backend.Any, ownerID, "", 20)
		require.NoError(t, err)
		require.Empty(t, data)
		require.Empty(t, offset)
	})

	t.Run("data in the response", func(t *testing.T) {
		expData := []*backend.TokenUnit{{ID: []byte{0, 1}, Symbol: "test", Amount: 42}}
		cli := createClient(t, expData)
		data, offset, err := cli.GetTokens(context.Background(), backend.Any, ownerID, "", 20)
		require.NoError(t, err)
		require.ElementsMatch(t, data, expData)
		require.Empty(t, offset)
	})
}

func Test_GetTokenTypes(t *testing.T) {
	t.Parallel()

	ownerID := test.RandomBytes(33)

	t.Run("valid request is built", func(t *testing.T) {
		cli := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					pubKeyHex := hexutil.Encode(ownerID)
					expURL := fmt.Sprintf("http://localhost/api/v1/kinds/all/types?creator=%s&limit=10", pubKeyHex)
					if r.URL.String() != expURL {
						t.Errorf("expected request URL %q, got %q", expURL, r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}

					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusOK)
					return w.Result(), nil
				},
			}},
		}

		data, offset, err := cli.GetTokenTypes(context.Background(), backend.Any, ownerID, "", 10)
		require.NoError(t, err)
		require.Empty(t, data)
		require.Empty(t, offset)
	})

	createClient := func(t *testing.T, respBody []*backend.TokenUnitType) *TokenBackend {
		return &TokenBackend{
			hc: &http.Client{
				Transport: &mockRoundTripper{
					do: func(r *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						if err := json.NewEncoder(w).Encode(respBody); err != nil {
							return nil, fmt.Errorf("failed to write response body: %v", err)
						}
						return w.Result(), nil
					},
				},
			},
		}
	}

	t.Run("no data in the response", func(t *testing.T) {
		cli := createClient(t, nil)
		data, offset, err := cli.GetTokenTypes(context.Background(), backend.Any, ownerID, "", 20)
		require.NoError(t, err)
		require.Empty(t, data)
		require.Empty(t, offset)
	})

	t.Run("data in the response", func(t *testing.T) {
		expData := []*backend.TokenUnitType{{ID: []byte{0, 1}, Symbol: "test"}}
		cli := createClient(t, expData)
		data, offset, err := cli.GetTokenTypes(context.Background(), backend.Any, ownerID, "", 20)
		require.NoError(t, err)
		require.ElementsMatch(t, data, expData)
		require.Empty(t, offset)
	})
}

func Test_GetTypeHierarchy(t *testing.T) {
	t.Parallel()

	typeID := test.RandomBytes(32)

	t.Run("valid request is built", func(t *testing.T) {
		cli := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "127.0.0.1:4321"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					typeIDStr := hexutil.Encode(typeID)
					expURL := fmt.Sprintf("http://127.0.0.1:4321/api/v1/types/%s/hierarchy", typeIDStr)
					if r.URL.String() != expURL {
						t.Errorf("expected request URL %q, got %q", expURL, r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}

					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusOK)
					return w.Result(), nil
				},
			}},
		}

		data, err := cli.GetTypeHierarchy(context.Background(), typeID)
		require.NoError(t, err)
		require.Empty(t, data)
	})

	createClient := func(t *testing.T, respBody []*backend.TokenUnitType) *TokenBackend {
		return &TokenBackend{
			hc: &http.Client{
				Transport: &mockRoundTripper{
					do: func(r *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						if respBody == nil {
							w.WriteHeader(http.StatusNotFound)
							w.WriteString(`{"message":"no type with this id"}`)
							return w.Result(), nil
						}

						err := json.NewEncoder(w).Encode(respBody)
						return w.Result(), err
					},
				},
			},
		}
	}

	t.Run("no type with given id", func(t *testing.T) {
		cli := createClient(t, nil)
		data, err := cli.GetTypeHierarchy(context.Background(), typeID)
		require.EqualError(t, err, `get token type hierarchy request failed: no type with this id: not found`)
		require.ErrorIs(t, err, sdk.ErrNotFound)
		require.Empty(t, data)
	})

	t.Run("type with given id exists", func(t *testing.T) {
		expData := []*backend.TokenUnitType{{ID: []byte{0, 1}, Symbol: "test"}}
		cli := createClient(t, expData)
		data, err := cli.GetTypeHierarchy(context.Background(), typeID)
		require.NoError(t, err)
		require.ElementsMatch(t, data, expData)
	})
}

func Test_GetTxProof(t *testing.T) {
	t.Parallel()

	unitID := test.RandomBytes(32)
	txHash := test.RandomBytes(32)

	createClient := func(t *testing.T, proof *sdk.Proof) *TokenBackend {
		return &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					unitIDHex := hexutil.Encode(unitID)
					txHashHex := hexutil.Encode(txHash)

					if r.URL.String() != fmt.Sprintf("http://localhost/api/v1/units/%s/transactions/%s/proof", unitIDHex, txHashHex) {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}

					w := httptest.NewRecorder()
					if proof == nil {
						w.WriteHeader(http.StatusNotFound)
						w.WriteString(`{"message":"no proof found"}`)
					} else {
						w.Header().Set(contentTypeHeader, applicationCbor)
						w.WriteHeader(http.StatusOK)
						if err := cbor.NewEncoder(w).Encode(proof); err != nil {
							return nil, fmt.Errorf("failed to write response body: %v", err)
						}
					}
					return w.Result(), nil
				},
			}},
		}
	}

	t.Run("valid proof returned", func(t *testing.T) {
		proof := &sdk.Proof{
			TxRecord: &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID, Attributes: []byte{0x00}}}},
			TxProof:  &types.TxProof{ /*TransactionsHash: txHash*/ },
		}

		proofFromClient, err := createClient(t, proof).GetTxProof(context.Background(), unitID, txHash)
		require.NoError(t, err)
		require.Equal(t, proof, proofFromClient)
	})

	t.Run("nil proof returned", func(t *testing.T) {
		proofFromClient, err := createClient(t, nil).GetTxProof(context.Background(), unitID, txHash)
		require.NoError(t, err)
		require.Nil(t, proofFromClient)
	})

	t.Run("404 returned", func(t *testing.T) {
		api := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusNotFound)
					return w.Result(), nil
				},
			}},
		}

		_, err := api.GetTxProof(context.Background(), unitID, txHash)
		require.ErrorContains(t, err, "get tx proof request failed")
	})

	t.Run("500 returned", func(t *testing.T) {
		api := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusInternalServerError)
					return w.Result(), nil
				},
			}},
		}

		_, err := api.GetTxProof(context.Background(), unitID, txHash)
		require.ErrorContains(t, err, "get tx proof request failed")
	})
}

func Test_PostTransactions(t *testing.T) {
	t.Parallel()

	ownerID := test.RandomBytes(33)

	t.Run("valid request is built", func(t *testing.T) {
		var receivedData sdk.Transactions

		cli := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					pubKeyHex := hexutil.Encode(ownerID)
					if r.URL.String() != `http://localhost/api/v1/transactions/`+pubKeyHex {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}

					defer r.Body.Close()
					buf, err := io.ReadAll(r.Body)
					if err != nil {
						return nil, fmt.Errorf("failed to read request body: %w", err)
					}

					if err := cbor.Unmarshal(buf, &receivedData); err != nil {
						return nil, fmt.Errorf("failed to decode request data: %w", err)
					}

					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusAccepted)
					return w.Result(), nil
				},
			}},
		}

		data := &sdk.Transactions{Transactions: []*types.TransactionOrder{randomTx(t, &tokens.CreateNonFungibleTokenTypeAttributes{Symbol: "test"})}}
		err := cli.PostTransactions(context.Background(), ownerID, data)
		require.NoError(t, err)
		require.Equal(t, data, &receivedData)
	})

	t.Run("invalid request", func(t *testing.T) {
		cli := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusBadRequest)
					if _, err := w.WriteString(`{"message":"something is wrong"}`); err != nil {
						t.Errorf("failed to write response body: %v", err)
					}
					return w.Result(), nil
				},
			}},
		}

		err := cli.PostTransactions(context.Background(), ownerID, &sdk.Transactions{})
		require.EqualError(t, err, `failed to send transactions: backend responded 400 Bad Request: something is wrong: invalid request`)
		require.ErrorIs(t, err, sdk.ErrInvalidRequest)
	})

	t.Run("processing tx failed", func(t *testing.T) {
		cli := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusAccepted)
					errs := map[string]string{"100001": "invalid id"}
					if err := json.NewEncoder(w).Encode(errs); err != nil {
						t.Errorf("failed to write response body: %v", err)
					}
					return w.Result(), nil
				},
			}},
		}

		data := &sdk.Transactions{}
		err := cli.PostTransactions(context.Background(), ownerID, data)
		require.EqualError(t, err, "failed to process some of the transactions:\n100001: invalid id")
	})

	t.Run("success", func(t *testing.T) {
		cli := &TokenBackend{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusAccepted)
					return w.Result(), nil
				},
			}},
		}

		data := &sdk.Transactions{}
		err := cli.PostTransactions(context.Background(), ownerID, data)
		require.NoError(t, err)
	})
}

func Test_New(t *testing.T) {
	t.Parallel()

	h := func(w http.ResponseWriter, r *http.Request) {
		if rp := path.Join(apiPathPrefix, "round-number"); rp != r.URL.Path {
			t.Errorf("expected request %q, got: %q", rp, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"roundNumber": "900"}`)
	}

	srv := httptest.NewServer(http.HandlerFunc(h))
	defer srv.Close()

	addr, err := url.Parse(srv.URL)
	require.NoError(t, err)

	cli := New(*addr, observability.Default(t))
	rn, err := cli.GetRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 900, rn)
}

func Test_extractOffsetMarker(t *testing.T) {
	t.Parallel()

	t.Run("no header", func(t *testing.T) {
		w := httptest.NewRecorder()
		marker, err := sdk.ExtractOffsetMarker(w.Result())
		require.NoError(t, err)
		require.Empty(t, marker)
	})

	t.Run("not matching the expected format", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Header().Set("Link", `unexpected header`)
		marker, err := sdk.ExtractOffsetMarker(w.Result())
		require.EqualError(t, err, "link header didn't result in expected match\nHeader: unexpected header\nmatches: []")
		require.Empty(t, marker)
	})

	t.Run("invalid link", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Header().Set("Link", `<://no.scheme>; rel="next"`)
		marker, err := sdk.ExtractOffsetMarker(w.Result())
		require.EqualError(t, err, `failed to parse Link header as URL: parse "://no.scheme": missing protocol scheme`)
		require.Empty(t, marker)
	})

	t.Run("offsetKey is not present", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Header().Set("Link", `<http://localhost/foo/bar>; rel="next"`)
		marker, err := sdk.ExtractOffsetMarker(w.Result())
		require.NoError(t, err)
		require.Empty(t, marker)
	})

	t.Run("offsetKey is present", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Header().Set("Link", `<http://localhost/foo/bar?offsetKey=ABC>; rel="next"`)
		marker, err := sdk.ExtractOffsetMarker(w.Result())
		require.NoError(t, err)
		require.Equal(t, "ABC", marker)
	})
}

func Test_Info(t *testing.T) {
	t.Parallel()

	createClient := func(t *testing.T, respBody string) *TokenBackend {
		t.Helper()
		return &TokenBackend{
			hc: &http.Client{
				Transport: &mockRoundTripper{
					do: func(r *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						if _, err := w.WriteString(respBody); err != nil {
							t.Errorf("failed to write response body: %v", err)
						}
						return w.Result(), nil
					},
				},
			},
		}
	}

	t.Run("ok", func(t *testing.T) {
		cli := createClient(t, `{"system_id": "00000002", "name": "tokens backend"}`)
		res, err := cli.GetInfo(context.Background())
		require.NoError(t, err)
		require.NotNil(t, res)
		require.LessOrEqual(t, "00000002", res.SystemID)
		require.LessOrEqual(t, "tokens backend", res.Name)
	})
}

func randomTx(t *testing.T, attr interface{}) *types.TransactionOrder {
	t.Helper()
	bytes, err := cbor.Marshal(attr)
	require.NoError(t, err, "failed to marshal tx attributes: %v", err)
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       tokens.DefaultSystemIdentifier,
			Attributes:     bytes,
			UnitID:         test.RandomBytes(32),
			ClientMetadata: &types.ClientMetadata{Timeout: 10},
		},
		OwnerProof: test.RandomBytes(32),
	}
	return tx
}

type mockRoundTripper struct {
	do func(*http.Request) (*http.Response, error)
}

func (mrt *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return mrt.do(req)
}
