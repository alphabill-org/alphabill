package fc

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewNoFeeCreditModule(t *testing.T) {
	noFees := NewNoFeeCreditModule()
	require.True(t, noFees.BuyGas(1) == math.MaxUint64)
	require.True(t, noFees.BuyGas(0) == math.MaxUint64)
	require.EqualValues(t, 0, noFees.CalculateCost(0))
	require.EqualValues(t, 0, noFees.CalculateCost(math.MaxUint64))
	require.NoError(t, noFees.IsCredible(nil, nil))
	require.Empty(t, noFees.TxHandlers())
}
