package types

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

func Test_TxExecutionContext(t *testing.T) {
	t.Run("gas", func(t *testing.T) {
		fees := mockFeeHandling{
			buyGas: func(tema uint64) uint64 { return tema * 10 },
			cost:   func(gas uint64) uint64 { return gas / 10 },
		}
		execCtx := NewExecutionContext(&mockSysInfo{}, &fees, 10)
		require.NotNil(t, execCtx)

		// constructor buys gas
		require.EqualValues(t, 100, execCtx.GasAvailable())

		// spend some
		require.NoError(t, execCtx.SpendGas(20))
		require.EqualValues(t, 80, execCtx.GasAvailable())
		require.EqualValues(t, 2, execCtx.CalculateCost())

		// attempt to overspend
		require.ErrorIs(t, execCtx.SpendGas(81), types.ErrOutOfGas)
	})

	t.Run("CurrentRound", func(t *testing.T) {
		const round = 197832
		info := &mockSysInfo{currentRound: func() uint64 { return round }}
		execCtx := NewExecutionContext(info, NewMockFeeModule(), 10)
		require.EqualValues(t, round, execCtx.CurrentRound())
	})

	t.Run("CommittedUC", func(t *testing.T) {
		uc := types.UnicityCertificate{TRHash: []byte{1, 6, 8, 0, 3, 4, 5}}
		info := &mockSysInfo{committedUC: func() *types.UnicityCertificate { return &uc }}
		execCtx := NewExecutionContext(info, NewMockFeeModule(), 10)
		require.Equal(t, &uc, execCtx.CommittedUC())
	})

	t.Run("GetUnit", func(t *testing.T) {
		expErr := fmt.Errorf("no such unit")
		unitID := types.UnitID{2, 4, 5, 6, 9, 0}

		info := &mockSysInfo{
			getUnit: func(id types.UnitID, committed bool) (state.Unit, error) {
				require.Equal(t, unitID, id)
				require.False(t, committed)
				return nil, expErr
			},
		}
		execCtx := NewExecutionContext(info, NewMockFeeModule(), 10)

		u, err := execCtx.GetUnit(unitID, false)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, u)

		info.getUnit = func(id types.UnitID, committed bool) (state.Unit, error) {
			require.Equal(t, unitID, id)
			require.True(t, committed)
			return &state.UnitV1{}, nil
		}
		u, err = execCtx.GetUnit(unitID, true)
		require.NoError(t, err)
		require.NotNil(t, u)
	})

	t.Run("ExtraArgument", func(t *testing.T) {
		execCtx := NewExecutionContext(mockSysInfo{}, NewMockFeeModule(), 10)
		data, err := execCtx.ExtraArgument()
		require.EqualError(t, err, `extra argument callback not assigned`)
		require.Nil(t, data)

		expData := []byte("some data")
		exCtx := execCtx.WithExArg(func() ([]byte, error) { return expData, nil })
		data, err = exCtx.ExtraArgument()
		require.NoError(t, err)
		require.Equal(t, expData, data)
	})
}
