package txsystem

import (
	"errors"
	"fmt"
	"hash"
	"math"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
)

const mockTxType = "mockTx-type"
const mockTxSystemID = types.SystemID(10)

type MockData struct {
	_     struct{} `cbor:",toarray"`
	Value uint64
}

func (t *MockData) Write(hasher hash.Hash) error {
	res, err := types.Cbor.Marshal(t)
	if err != nil {
		return fmt.Errorf("test data serialization error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}
func (t *MockData) SummaryValueInput() uint64 {
	return t.Value
}
func (t *MockData) Copy() types.UnitData {
	return &MockData{Value: t.Value}
}

func Test_NewGenericTxSystem(t *testing.T) {
	t.Run("system ID param is mandatory", func(t *testing.T) {
		txSys, err := NewGenericTxSystem(0, nil, nil, nil, nil)
		require.Nil(t, txSys)
		require.EqualError(t, err, `system ID must be assigned`)
	})
	t.Run("observe must not be nil", func(t *testing.T) {
		txSys, err := NewGenericTxSystem(mockTxSystemID, nil, nil, nil)
		require.Nil(t, txSys)
		require.EqualError(t, err, "observe must not be nil")
	})
	t.Run("success", func(t *testing.T) {
		obs := observability.Default(t)
		txSys, err := NewGenericTxSystem(
			mockTxSystemID,
			nil,
			nil,
			obs,
		)
		require.NoError(t, err)
		require.EqualValues(t, mockTxSystemID, txSys.systemIdentifier)
		require.NotNil(t, txSys.log)
		// no fees
		require.Nil(t, txSys.fees)
	})
}

func Test_GenericTxSystem_Execute(t *testing.T) {
	t.Run("tx order is validated", func(t *testing.T) {
		txSys := NewTestGenericTxSystem(t, nil)
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID+1),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}))
		md, err := txSys.Execute(txo)
		require.ErrorIs(t, err, ErrInvalidSystemIdentifier)
		require.Nil(t, md)
	})

	t.Run("no executor for the tx type", func(t *testing.T) {
		txSys := NewTestGenericTxSystem(t, nil)
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		// no modules, no tx handlers
		md, err := txSys.Execute(txo)
		require.NotNil(t, md)
		require.EqualValues(t, types.TxStatusFailed, md.SuccessIndicator)
		require.Nil(t, err)
	})

	t.Run("tx validate returns error", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(nil)
		m.ValidateError = expErr
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		md, err := txSys.Execute(txo)
		require.NotNil(t, md)
		require.EqualValues(t, types.TxStatusFailed, md.SuccessIndicator)
		require.Nil(t, err)
	})

	t.Run("tx execute returns error", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(expErr)
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		md, err := txSys.Execute(txo)
		require.NotNil(t, md)
		require.EqualValues(t, types.TxStatusFailed, md.SuccessIndicator)
		require.Nil(t, err)
	})

	t.Run("locked unit - unlock fails", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(expErr)
		unitID := []byte{1, 2, 3}
		fcrID := types.NewUnitID(33, nil, []byte{1}, []byte{0xff})
		txSys := NewTestGenericTxSystem(t,
			[]txtypes.Module{m},
			withStateUnit(fcrID,
				templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10}, nil),
			withStateUnit(unitID,
				templates.AlwaysTrueBytes(),
				&MockData{Value: 1}, newMockLockTx(t,
					transaction.WithSystemID(mockTxSystemID),
					transaction.WithPayloadType(mockTxType),
					transaction.WithAttributes(MockTxAttributes{}),
					transaction.WithClientMetadata(&types.ClientMetadata{
						Timeout:           1000000,
						MaxTransactionFee: 1,
					}),
					transaction.WithStateLock(&types.StateLock{
						ExecutionPredicate: templates.AlwaysTrueBytes(),
						RollbackPredicate:  templates.AlwaysTrueBytes(),
					}),
				),
			),
		)
		txo := transaction.NewTransactionOrder(t,
			transaction.WithUnitID(unitID),
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
		require.EqualValues(t, types.TxStatusFailed, md.SuccessIndicator)
	})

	t.Run("locked unit - unlocked, but execution fails", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(expErr)
		unitID := []byte{1, 2, 3}
		fcrID := types.NewUnitID(33, nil, []byte{1}, []byte{0xff})
		txSys := NewTestGenericTxSystem(t,
			[]txtypes.Module{m},
			withStateUnit(fcrID,
				templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10}, nil),
			withStateUnit(unitID,
				templates.AlwaysTrueBytes(),
				&MockData{Value: 1}, newMockLockTx(t,
					transaction.WithSystemID(mockTxSystemID),
					transaction.WithPayloadType(mockTxType),
					transaction.WithAttributes(MockTxAttributes{}),
					transaction.WithClientMetadata(&types.ClientMetadata{
						Timeout: 1000000,
					}),
					transaction.WithStateLock(&types.StateLock{
						ExecutionPredicate: templates.AlwaysTrueBytes(),
						RollbackPredicate:  templates.AlwaysTrueBytes(),
					}),
				),
			),
		)
		txo := transaction.NewTransactionOrder(t,
			transaction.WithUnitID(unitID),
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				FeeCreditRecordID: fcrID,
				MaxTransactionFee: 10,
			}),
			transaction.WithUnlockProof([]byte{byte(StateUnlockExecute)}),
		)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
		require.EqualValues(t, types.TxStatusFailed, md.SuccessIndicator)
	})

	t.Run("lock fails - validate fails", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(nil)
		m.ValidateError = expErr
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
			transaction.WithStateLock(&types.StateLock{
				ExecutionPredicate: templates.AlwaysTrueBytes(),
				RollbackPredicate:  templates.AlwaysTrueBytes()}),
		)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
		require.EqualValues(t, types.TxStatusFailed, md.SuccessIndicator)
	})

	t.Run("lock fails - state lock invalid", func(t *testing.T) {
		m := NewMockTxModule(nil)
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
			transaction.WithStateLock(&types.StateLock{}),
		)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
		require.EqualValues(t, types.TxStatusFailed, md.SuccessIndicator)
	})

	t.Run("lock success", func(t *testing.T) {
		m := NewMockTxModule(nil)
		unitID := []byte{2}
		fcrID := types.NewUnitID(33, nil, []byte{1}, []byte{0xff})
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m},
			withStateUnit(unitID,
				templates.AlwaysTrueBytes(),
				&MockData{Value: 1}, nil),
			withStateUnit(fcrID,
				templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10}, nil))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithUnitID(unitID),
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				FeeCreditRecordID: fcrID,
				MaxTransactionFee: 1,
			}),
			transaction.WithStateLock(&types.StateLock{
				ExecutionPredicate: templates.AlwaysTrueBytes(),
				RollbackPredicate:  templates.AlwaysTrueBytes()}),
		)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
	})

	t.Run("success", func(t *testing.T) {
		m := NewMockTxModule(nil)
		fcrID := types.NewUnitID(33, nil, []byte{1}, []byte{0xff})
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m},
			withStateUnit(fcrID,
				templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10}, nil))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				FeeCreditRecordID: fcrID,
				MaxTransactionFee: 1,
			}),
		)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
	})
}

func Test_GenericTxSystem_validateGenericTransaction(t *testing.T) {
	// create valid order (in the sense of basic checks performed by the generic
	// tx system) for "txs" transaction system
	createTxOrder := func(txs *GenericTxSystem) *types.TransactionOrder {
		return &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: txs.systemIdentifier,
				ClientMetadata: &types.ClientMetadata{
					Timeout: txs.currentRoundNumber + 1,
				},
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		// this (also) tests that our helper functions do create valid
		// tx system and tx order combination (other tests depend on that)
		txSys := NewTestGenericTxSystem(t, nil)
		txo := createTxOrder(txSys)
		require.NoError(t, txSys.validateGenericTransaction(txo))
	})

	t.Run("system ID is checked", func(t *testing.T) {
		txSys := NewTestGenericTxSystem(t, nil)
		txo := createTxOrder(txSys)
		txo.Payload.SystemID = txSys.systemIdentifier + 1
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrInvalidSystemIdentifier)
	})

	t.Run("timeout is checked", func(t *testing.T) {
		txSys := NewTestGenericTxSystem(t, nil)
		txo := createTxOrder(txSys)

		txSys.currentRoundNumber = txo.Timeout()
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrTransactionExpired)
		txSys.currentRoundNumber = txo.Timeout() + 1
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrTransactionExpired)
		txSys.currentRoundNumber = math.MaxUint64
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrTransactionExpired)
	})
}

type MockTxAttributes struct {
	_     struct{} `cbor:",toarray"`
	Value uint64
}

func newMockLockTx(t *testing.T, option ...transaction.Option) []byte {
	txo := transaction.NewTransactionOrder(t, option...)
	txBytes, err := cbor.Marshal(txo)
	require.NoError(t, err)
	return txBytes
}

type MockModule struct {
	ValidateError error
	Result        error
}

func NewMockTxModule(wantErr error) *MockModule {
	return &MockModule{Result: wantErr}
}

func (mm MockModule) mockValidateTx(tx *types.TransactionOrder, _ *MockTxAttributes, _ txtypes.ExecutionContext) (err error) {
	return mm.ValidateError
}
func (mm MockModule) mockExecuteTx(tx *types.TransactionOrder, _ *MockTxAttributes, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	if mm.Result != nil {
		return &types.ServerMetadata{SuccessIndicator: types.TxStatusFailed}, mm.Result
	}
	return &types.ServerMetadata{ActualFee: 0, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (mm MockModule) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		mockTxType: txtypes.NewTxHandler[MockTxAttributes](mm.mockValidateTx, mm.mockExecuteTx),
	}
}

type txSystemTestOption func(m *GenericTxSystem) error

func withStateUnit(unitID []byte, bearer types.PredicateBytes, data types.UnitData, lock []byte) txSystemTestOption {
	return func(m *GenericTxSystem) error {
		return m.state.Apply(state.AddUnitWithLock(unitID, bearer, data, lock))
	}
}

func withTrustBase(tb types.RootTrustBase) txSystemTestOption {
	return func(m *GenericTxSystem) error {
		m.trustBase = tb
		return nil
	}
}

func withCurrentRound(round uint64) txSystemTestOption {
	return func(m *GenericTxSystem) error {
		m.currentRoundNumber = round
		return nil
	}
}

func NewTestGenericTxSystem(t *testing.T, modules []txtypes.Module, opts ...txSystemTestOption) *GenericTxSystem {
	txSys := defaultTestConfiguration(t, modules)
	// apply test overrides
	for _, opt := range opts {
		require.NoError(t, opt(txSys))
	}
	return txSys
}

func defaultTestConfiguration(t *testing.T, modules []txtypes.Module) *GenericTxSystem {
	// default configuration has no fee handling
	txSys, err := NewGenericTxSystem(mockTxSystemID, nil, modules, observability.Default(t))
	require.NoError(t, err)
	return txSys
}
