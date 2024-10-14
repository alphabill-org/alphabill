package exec_context

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

type MockExecContext struct {
	Tx            *types.TransactionOrder
	Unit          *state.Unit
	RootTrustBase types.RootTrustBase
	RoundNumber   uint64
	GasRemaining  uint64
	mockErr       error
	customData    []byte
}

func (m *MockExecContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	if m.mockErr != nil {
		return nil, m.mockErr
	}
	// return avl.ErrNotFound if unit does not exist to be consistent with actual implementation
	if m.Unit == nil {
		return nil, avl.ErrNotFound
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

func (m *MockExecContext) TransactionOrder() (*types.TransactionOrder, error) {
	if m.mockErr != nil {
		return nil, m.mockErr
	}
	return m.Tx, nil
}

func (m *MockExecContext) GetData() []byte {
	return m.customData
}

func (m *MockExecContext) SetData(data []byte) {
	m.customData = data
}

type TestOption func(*MockExecContext)

func WithCurrentRound(round uint64) TestOption {
	return func(m *MockExecContext) {
		m.RoundNumber = round
	}
}

func WithUnit(unit *state.Unit) TestOption {
	return func(m *MockExecContext) {
		m.Unit = unit
	}
}

func WithData(data []byte) TestOption {
	return func(m *MockExecContext) {
		m.customData = data
	}
}

func WithErr(err error) TestOption {
	return func(m *MockExecContext) {
		m.mockErr = err
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

func NewMockExecutionContext(options ...TestOption) txtypes.ExecutionContext {
	execCtx := &MockExecContext{}
	for _, o := range options {
		o(execCtx)
	}
	return execCtx
}
