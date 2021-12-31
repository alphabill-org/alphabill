package tree

import (
	"testing"

	"github.com/holiman/uint256"

	"github.com/stretchr/testify/require"
)

func TestEmpty(t *testing.T) {
	tr := New()
	require.NotNil(t, tr)
	require.Nil(t, tr.GetRootHash())
	require.Nil(t, tr.GetSummaryValue())
}

func TestOneItem(t *testing.T) {
	tr := New()

	err := tr.Set(uint256.NewInt(0), Predicate{1, 2, 3}, uint64(100))
	require.NoError(t, err)

	require.NotNil(t, tr.GetRootHash())

	genSum := tr.GetSummaryValue()
	sum, ok := genSum.(uint64)
	require.True(t, ok, "should be type of uint64")
	require.Equal(t, uint64(100), sum)
}
