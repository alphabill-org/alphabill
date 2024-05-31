package types

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/stretchr/testify/require"
)

type stateInfo struct {
}

func (s *stateInfo) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return nil, fmt.Errorf("unit does not exist")
}

func (s *stateInfo) CurrentRound() uint64 {
	return 1
}

func Test_newExecutionContext(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		_, verifier := testsig.CreateSignerAndVerifier(t)
		tb := testtb.NewTrustBase(t, verifier)
		info := &stateInfo{}
		execCtx := NewExecutionContext(info, tb, 10)
		require.NotNil(t, execCtx)
		require.EqualValues(t, 1, execCtx.CurrentRound())
		tb, err := execCtx.TrustBase(0)
		require.NoError(t, err)
		require.NotNil(t, tb)
		u, err := execCtx.GetUnit(types.UnitID{2}, false)
		require.Error(t, err)
		require.Nil(t, u)
	})
}

func TestTxExecutionContext_CalculateCost(t *testing.T) {
	type fields struct {
		initialGas   uint64
		remainingGas uint64
	}
	tests := []struct {
		fields fields
		want   uint64
	}{
		{
			fields: fields{initialGas: 10*GasUnitsPerTema - 1, remainingGas: 0},
			want:   10,
		},
		{
			fields: fields{initialGas: GasUnitsPerTema, remainingGas: 0},
			want:   1,
		},
		{
			fields: fields{initialGas: GasUnitsPerTema - 2, remainingGas: 0},
			want:   1,
		},
		{
			fields: fields{initialGas: GasUnitsPerTema / 2, remainingGas: 0},
			want:   1,
		},
		{
			fields: fields{initialGas: GasUnitsPerTema, remainingGas: GasUnitsPerTema},
			want:   0,
		},
		{
			fields: fields{initialGas: GasUnitsPerTema, remainingGas: GasUnitsPerTema/2 + 1},
			want:   0,
		},
	}
	t.Run("test conversion", func(t *testing.T) {
		for i, tt := range tests {
			ec := &TxExecutionContext{
				initialGas:   tt.fields.initialGas,
				remainingGas: tt.fields.remainingGas,
			}
			if got := ec.CalculateCost(); got != tt.want {
				t.Errorf("CalculateCost(%v) = %v, want %v", i, got, tt.want)
			}
		}
	})
}
