package txsystem

import (
	"fmt"
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/txsystem/testutils/transaction"

	"github.com/alphabill-org/alphabill/state"
	"github.com/stretchr/testify/require"
)

type IdTestData struct {
	_       struct{} `cbor:",toarray"`
	Value   uint64
	Counter uint64
}

func (t *IdTestData) Write(hasher hash.Hash) error {
	res, err := types.Cbor.Marshal(t)
	if err != nil {
		return fmt.Errorf("test data serialization error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (t *IdTestData) SummaryValueInput() uint64 {
	return t.Value
}

func (t *IdTestData) Copy() types.UnitData {
	return &IdTestData{Value: t.Value, Counter: t.Counter}
}

func (t *IdTestData) IncrementCounter() {}

func Test_IdentityModule(t *testing.T) {
	t.Run("err - state is nil", func(t *testing.T) {
		m, err := NewIdentityModule(nil)
		require.EqualError(t, err, "state is nil")
		require.Nil(t, m)
	})
}

func Test_IdentityModule_validateIdentityTx(t *testing.T) {
	t.Run("unit not found", func(t *testing.T) {
		m := newTestIdentityModule(t)
		exeCtx := &TxExecutionContext{}
		err := m.validateIdentityTx(&types.TransactionOrder{}, &IdentityAttributes{}, exeCtx)
		require.EqualError(t, err, "unable to fetch the unit: item  does not exist: not found")
	})
	t.Run("unit not found", func(t *testing.T) {
		m := newTestIdentityModule(t)
		exeCtx := &TxExecutionContext{}
		err := m.validateIdentityTx(nil, nil, exeCtx)
		require.EqualError(t, err, "tx is nil")
	})
	t.Run("state lock released, tx does nothing", func(t *testing.T) {
		unitID := types.NewUnitID(33, nil, []byte{0x02}, []byte{1})
		tx := transaction.NewTransactionOrder(t,
			transaction.WithUnitID(unitID),
			transaction.WithPayloadType(TxIdentity),
			transaction.WithAttributes(&IdentityAttributes{}),
			transaction.WithStateLock(&types.StateLock{
				ExecutionPredicate: templates.AlwaysTrueBytes(),
				RollbackPredicate:  nil,
			}),
			transaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 10}),
		)
		m := newTestIdentityModule(t, withStateUnit(unitID, templates.AlwaysTrueBytes(), &IdTestData{Value: 1}))
		exeCtx := &TxExecutionContext{}
		err := m.validateIdentityTx(tx, nil, exeCtx)
		require.NoError(t, err)
	})
	t.Run("bearer not validated", func(t *testing.T) {
		unitID := types.NewUnitID(33, nil, []byte{0x02}, []byte{1})
		tx := transaction.NewTransactionOrder(t,
			transaction.WithUnitID(unitID),
			transaction.WithPayloadType(TxIdentity),
			transaction.WithAttributes(&IdentityAttributes{}),
			transaction.WithStateLock(&types.StateLock{
				ExecutionPredicate: templates.AlwaysTrueBytes(),
				RollbackPredicate:  nil,
			}),
			transaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 10}),
		)
		m := newTestIdentityModule(t, withStateUnit(unitID, templates.AlwaysFalseBytes(), &IdTestData{Value: 1}))
		exeCtx := &TxExecutionContext{}
		err := m.validateIdentityTx(tx, nil, exeCtx)
		require.ErrorContains(t, err, "invalid owner proof")
	})
	t.Run("id tx locks state via SetStateLock", func(t *testing.T) {
		unitID := types.NewUnitID(33, nil, []byte{0x02}, []byte{1})
		m := newTestIdentityModule(t, withStateUnit(unitID, templates.AlwaysTrueBytes(), &IdTestData{Value: 1}))
		exeCtx := &TxExecutionContext{}
		tx := transaction.NewTransactionOrder(t,
			transaction.WithUnitID(unitID),
			transaction.WithPayloadType(TxIdentity),
			transaction.WithAttributes(&IdentityAttributes{}),
			transaction.WithStateLock(&types.StateLock{
				ExecutionPredicate: templates.AlwaysFalseBytes(),
				RollbackPredicate:  nil,
			}),
			transaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 10}),
		)
		err := m.validateIdentityTx(tx, nil, exeCtx)
		require.NoError(t, err)
	})
}

type identityModuleOption func(m *IdentityModule) error

func withStateUnit(unitID []byte, bearer types.PredicateBytes, data types.UnitData) identityModuleOption {
	return func(m *IdentityModule) error {
		return m.state.Apply(state.AddUnit(unitID, bearer, data))
	}
}

func newTestIdentityModule(t *testing.T, opts ...identityModuleOption) *IdentityModule {
	s := state.NewEmptyState()
	module, err := NewIdentityModule(s)
	require.NoError(t, err)
	for _, opt := range opts {
		require.NoError(t, opt(module))
	}
	return module
}
