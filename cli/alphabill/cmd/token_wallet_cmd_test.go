package cmd

import (
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/stretchr/testify/require"
	"testing"
)

type accountManagerMock struct {
	keyHash       []byte
	recordedIndex uint64
}

func (a *accountManagerMock) GetAccountKey(accountIndex uint64) (*wallet.AccountKey, error) {
	a.recordedIndex = accountIndex
	return &wallet.AccountKey{PubKeyHash: &wallet.KeyHashes{Sha256: a.keyHash}}, nil
}

func TestParsePredicateClause(t *testing.T) {
	predicate, err := parsePredicateClause("true", nil)
	require.NoError(t, err)
	require.Equal(t, script.PredicateAlwaysTrue(), predicate)

	predicate, err = parsePredicateClause("false", nil)
	require.NoError(t, err)
	require.Equal(t, script.PredicateAlwaysFalse(), predicate)

	predicate, err = parsePredicateClause("", nil)
	require.ErrorContains(t, err, "invalid predicate clause")

	predicate, err = parsePredicateClause("ptpkh:", nil)
	require.Error(t, err)

	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	predicate, err = parsePredicateClause("ptpkh", mock)
	require.NoError(t, err)
	require.Equal(t, uint64(1), mock.recordedIndex)
	require.Equal(t, script.PredicatePayToPublicKeyHashDefault(mock.keyHash), predicate)

	predicate, err = parsePredicateClause("ptpkh:2", mock)
	require.NoError(t, err)
	require.Equal(t, uint64(2), mock.recordedIndex)
	require.Equal(t, script.PredicatePayToPublicKeyHashDefault(mock.keyHash), predicate)

	predicate, err = parsePredicateClause("ptpkh:0X", nil)
	require.ErrorContains(t, err, "invalid predicate clause")

	predicate, err = parsePredicateClause("ptpkh:0x0102", nil)
	require.NoError(t, err)
	require.Equal(t, script.PredicatePayToPublicKeyHashDefault(mock.keyHash), predicate)

}
