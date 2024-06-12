package exec_context

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"github.com/stretchr/testify/require"
)

type MockExecContext struct {
	Tx            *types.TransactionOrder
	Unit          *state.Unit
	RootTrustBase types.RootTrustBase
	RoundNumber   uint64
	GasRemaining  uint64
	mockErr       error
}

func (m *MockExecContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	if m.mockErr != nil {
		return nil, m.mockErr
	}
	return m.Unit, nil
}

func (m *MockExecContext) CurrentRound() uint64 { return m.RoundNumber }

func (m *MockExecContext) TrustBase(epoch uint64) (types.RootTrustBase, error) {
	if m.mockErr != nil {
		return nil, m.mockErr
	}
	return m.RootTrustBase, nil
}

// until AB-1012 gets resolved we need this hack to get correct payload bytes.
func (m *MockExecContext) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	return txo.PayloadBytes()
}

type TestOption func(*MockExecContext) error

func WithCurrentRound(round uint64) TestOption {
	return func(m *MockExecContext) error {
		m.RoundNumber = round
		return nil
	}
}

func (m *MockExecContext) GasAvailable() uint64 {
	return m.GasRemaining
}

func (m *MockExecContext) SpendGas(gas uint64) error {
	return m.mockErr
}

func (m *MockExecContext) CalculateCost() uint64 {
	//gasUsed := ec.initialGas - ec.remainingGas
	return 1 // (gasUsed + GasUnitsPerTema/2) / GasUnitsPerTema
}

func NewMockExecutionContext(t *testing.T, options ...TestOption) txtypes.ExecutionContext {
	execCtx := &MockExecContext{}
	for _, o := range options {
		require.NoError(t, o(execCtx))
	}
	return execCtx
}
