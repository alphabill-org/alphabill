package tokens

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/stretchr/testify/require"
)

func Test_Load(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/round-number", r.URL.Path)
		_, err := fmt.Fprint(w, `{"roundNumber": 42}`)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w, err := Load(srv.URL, nil)
	require.NoError(t, err)

	rn, err := w.getRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 42, rn)
}

func Test_ListTokens(t *testing.T) {
	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind twb.Kind, _ twb.PubKey, _ string, _ int) ([]twb.TokenUnit, string, error) {
			fungible := []twb.TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: twb.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: twb.Fungible,
				},
			}
			nfts := []twb.TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: twb.NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: twb.NonFungible,
				},
			}
			switch kind {
			case twb.Fungible:
				return fungible, "", nil
			case twb.NonFungible:
				return nfts, "", nil
			case twb.Any:
				return append(fungible, nfts...), "", nil
			}
			return nil, "", fmt.Errorf("invalid kind")
		},
	}

	tw := initTestWallet(t, be)
	tokens, err := tw.ListTokens(context.Background(), twb.Any, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokens[1], 4)

	tokens, err = tw.ListTokens(context.Background(), twb.Fungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokens[1], 2)

	tokens, err = tw.ListTokens(context.Background(), twb.NonFungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokens[1], 2)
}

func Test_ListTokens_offset(t *testing.T) {
	allTokens := []twb.TokenUnit{
		{
			ID:     test.RandomBytes(32),
			Kind:   twb.Fungible,
			Symbol: "1",
		},
		{
			ID:     test.RandomBytes(32),
			Kind:   twb.Fungible,
			Symbol: "2",
		},
		{
			ID:     test.RandomBytes(32),
			Kind:   twb.Fungible,
			Symbol: "3",
		},
	}

	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind twb.Kind, _ twb.PubKey, offsetKey string, _ int) ([]twb.TokenUnit, string, error) {
			return getSubarray(allTokens, offsetKey)
		},
	}

	tw := initTestWallet(t, be)
	tokens, err := tw.ListTokens(context.Background(), twb.Any, AllAccounts)
	tokensForAccount := tokens[1]
	require.NoError(t, err)
	require.Len(t, tokensForAccount, len(allTokens))
	dereferencedTokens := make([]twb.TokenUnit, len(tokensForAccount))
	for i := range tokensForAccount {
		dereferencedTokens[i] = *tokensForAccount[i]
	}
	require.Equal(t, allTokens, dereferencedTokens)
}

func Test_ListTokenTypes(t *testing.T) {
	be := &mockTokenBackend{
		getTokenTypes: func(ctx context.Context, kind twb.Kind, _ twb.PubKey, _ string, _ int) ([]twb.TokenUnitType, string, error) {
			fungible := []twb.TokenUnitType{
				{
					ID:   test.RandomBytes(32),
					Kind: twb.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: twb.Fungible,
				},
			}
			nfts := []twb.TokenUnitType{
				{
					ID:   test.RandomBytes(32),
					Kind: twb.NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: twb.NonFungible,
				},
			}
			switch kind {
			case twb.Fungible:
				return fungible, "", nil
			case twb.NonFungible:
				return nfts, "", nil
			case twb.Any:
				return append(fungible, nfts...), "", nil
			}
			return nil, "", fmt.Errorf("invalid kind")
		},
	}

	tw := initTestWallet(t, be)
	types, err := tw.ListTokenTypes(context.Background(), twb.Any)
	require.NoError(t, err)
	require.Len(t, types, 4)

	types, err = tw.ListTokenTypes(context.Background(), twb.Fungible)
	require.NoError(t, err)
	require.Len(t, types, 2)

	types, err = tw.ListTokenTypes(context.Background(), twb.NonFungible)
	require.NoError(t, err)
	require.Len(t, types, 2)
}

func Test_ListTokenTypes_offset(t *testing.T) {
	allTypes := []twb.TokenUnitType{
		{
			ID:     test.RandomBytes(32),
			Symbol: "1",
			Kind:   twb.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "2",
			Kind:   twb.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "3",
			Kind:   twb.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "4",
			Kind:   twb.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "5",
			Kind:   twb.Fungible,
		},
	}
	be := &mockTokenBackend{
		getTokenTypes: func(ctx context.Context, _ twb.Kind, _ twb.PubKey, offsetKey string, _ int) ([]twb.TokenUnitType, string, error) {
			return getSubarray(allTypes, offsetKey)
		},
	}

	tw := initTestWallet(t, be)
	types, err := tw.ListTokenTypes(context.Background(), twb.Any)
	require.NoError(t, err)
	require.Len(t, types, len(allTypes))
	dereferencedTypes := make([]twb.TokenUnitType, len(types))
	for i := range types {
		dereferencedTypes[i] = *types[i]
	}
	require.Equal(t, allTypes, dereferencedTypes)
}

func initTestWallet(t *testing.T, backend TokenBackend) *Wallet {
	t.Helper()
	txs, err := ttxs.New()
	require.NoError(t, err)

	return &Wallet{
		txs:     txs,
		am:      initAccountManager(t),
		backend: backend,
	}
}

func initAccountManager(t *testing.T) account.Manager {
	t.Helper()
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(""))
	return am
}

type mockTokenBackend struct {
	getTokens        func(ctx context.Context, kind twb.Kind, owner twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnit, string, error)
	getTokenTypes    func(ctx context.Context, kind twb.Kind, creator twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnitType, string, error)
	getRoundNumber   func(ctx context.Context) (uint64, error)
	postTransactions func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error
}

func (m *mockTokenBackend) GetTokens(ctx context.Context, kind twb.Kind, owner twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnit, string, error) {
	if m.getTokens != nil {
		return m.getTokens(ctx, kind, owner, offsetKey, limit)
	}
	return nil, "", fmt.Errorf("GetTokens not implemented")
}

func (m *mockTokenBackend) GetTokenTypes(ctx context.Context, kind twb.Kind, creator twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnitType, string, error) {
	if m.getTokenTypes != nil {
		return m.getTokenTypes(ctx, kind, creator, offsetKey, limit)
	}
	return nil, "", fmt.Errorf("GetTokenTypes not implemented")
}

func (m *mockTokenBackend) GetRoundNumber(ctx context.Context) (uint64, error) {
	if m.getRoundNumber != nil {
		return m.getRoundNumber(ctx)
	}
	return 0, fmt.Errorf("GetRoundNumber not implemented")
}

func (m *mockTokenBackend) PostTransactions(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
	if m.postTransactions != nil {
		return m.postTransactions(ctx, pubKey, txs)
	}
	return fmt.Errorf("PostTransactions not implemented")
}

func getSubarray[T interface{}](array []T, offsetKey string) ([]T, string, error) {
	defaultLimit := 2
	offset := 0
	var err error

	if offsetKey != "" {
		offset, err = strconv.Atoi(offsetKey)
		if err != nil {
			return nil, "", err
		}
	}
	subarray := array[offset:util.Min(offset+defaultLimit, len(array))]
	offset += defaultLimit
	if offset >= len(array) {
		offsetKey = ""
	} else {
		offsetKey = strconv.Itoa(offset)
	}
	return subarray, offsetKey, nil
}
