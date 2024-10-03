package txsystem

import (
	"errors"
	"fmt"
	"hash"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

const mockTxType uint16 = 22
const mockNetworkID types.NetworkID = 5
const mockTxSystemID types.SystemID = 10

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
	validPDR := types.PartitionDescriptionRecord{
		NetworkIdentifier: mockNetworkID,
		SystemIdentifier:  mockTxSystemID,
		TypeIdLen:         8,
		UnitIdLen:         256,
		T2Timeout:         2500 * time.Millisecond,
	}
	require.NoError(t, validPDR.IsValid())

	t.Run("system ID param is mandatory", func(t *testing.T) {
		pdr := validPDR
		pdr.SystemIdentifier = 0
		txSys, err := NewGenericTxSystem(pdr, types.ShardID{}, nil, nil, nil, nil)
		require.Nil(t, txSys)
		require.EqualError(t, err, `invalid Partition Description: invalid system identifier: 00000000`)
	})

	t.Run("observability must not be nil", func(t *testing.T) {
		txSys, err := NewGenericTxSystem(validPDR, types.ShardID{}, nil, nil, nil)
		require.Nil(t, txSys)
		require.EqualError(t, err, "observability must not be nil")
	})

	t.Run("success", func(t *testing.T) {
		obs := observability.Default(t)
		txSys, err := NewGenericTxSystem(
			validPDR,
			types.ShardID{},
			nil,
			nil,
			obs,
		)
		require.NoError(t, err)
		require.EqualValues(t, mockTxSystemID, txSys.pdr.SystemIdentifier)
		require.NotNil(t, txSys.log)
		require.NotNil(t, txSys.fees)
		// default is no fee handling, which will give you a huge gas budget
		require.True(t, txSys.fees.BuyGas(1) == math.MaxUint64)
	})
}

func Test_GenericTxSystem_Execute(t *testing.T) {
	t.Run("tx order is validated", func(t *testing.T) {
		txSys := NewTestGenericTxSystem(t, nil)
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID+1),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}))
		md, err := txSys.Execute(txo)
		require.ErrorIs(t, err, ErrInvalidSystemIdentifier)
		require.Nil(t, md)
	})

	t.Run("no executor for the tx type", func(t *testing.T) {
		txSys := NewTestGenericTxSystem(t, nil)
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		// no modules, no tx handlers
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
		require.EqualValues(t, types.TxStatusFailed, md.SuccessIndicator)
	})

	t.Run("tx validate returns error", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(nil)
		m.ValidateError = expErr
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithTransactionType(mockTxType),
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

	t.Run("tx validate returns out of gas", func(t *testing.T) {
		expErr := types.ErrOutOfGas
		m := NewMockTxModule(nil)
		m.ValidateError = expErr
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
		require.EqualValues(t, types.TxErrOutOfGas, md.SuccessIndicator)
	})

	t.Run("tx execute returns error", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(expErr)
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(mockTxSystemID),
			transaction.WithTransactionType(mockTxType),
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

	t.Run("locked unit - unlock fails", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(expErr)
		unitID := test.RandomBytes(33)
		fcrID := types.NewUnitID(33, nil, []byte{1}, []byte{0xff})
		txSys := NewTestGenericTxSystem(t,
			[]txtypes.Module{m},
			withStateUnit(fcrID,
				templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10}, nil),
			withStateUnit(unitID,
				templates.AlwaysTrueBytes(),
				&MockData{Value: 1}, newMockLockTx(t,
					transaction.WithSystemID(mockTxSystemID),
					transaction.WithTransactionType(mockTxType),
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
			transaction.WithTransactionType(mockTxType),
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
		unitID := test.RandomBytes(33)
		fcrID := types.NewUnitID(33, nil, []byte{1}, []byte{0xff})
		txSys := NewTestGenericTxSystem(t,
			[]txtypes.Module{m},
			withStateUnit(fcrID,
				templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10}, nil),
			withStateUnit(unitID,
				templates.AlwaysTrueBytes(),
				&MockData{Value: 1}, newMockLockTx(t,
					transaction.WithSystemID(mockTxSystemID),
					transaction.WithTransactionType(mockTxType),
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
			transaction.WithTransactionType(mockTxType),
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
			transaction.WithTransactionType(mockTxType),
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
			transaction.WithTransactionType(mockTxType),
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
		unitID := test.RandomBytes(33)
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
			transaction.WithTransactionType(mockTxType),
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
			transaction.WithTransactionType(mockTxType),
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
			Payload: types.Payload{
				NetworkID: txs.pdr.NetworkIdentifier,
				SystemID:  txs.pdr.SystemIdentifier,
				UnitID:    make(types.UnitID, 33),
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
		txo.SystemID = txSys.pdr.SystemIdentifier + 1
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

type MockTxAuthProof struct {
	_          struct{} `cbor:",toarray"`
	OwnerProof []byte
}

func newMockLockTx(t *testing.T, option ...transaction.Option) []byte {
	txo := transaction.NewTransactionOrder(t, option...)
	txBytes, err := types.Cbor.Marshal(txo)
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

func (mm MockModule) mockValidateTx(tx *types.TransactionOrder, _ *MockTxAttributes, _ *MockTxAuthProof, _ txtypes.ExecutionContext) (err error) {
	return mm.ValidateError
}

func (mm MockModule) mockExecuteTx(tx *types.TransactionOrder, _ *MockTxAttributes, _ *MockTxAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	if mm.Result != nil {
		return &types.ServerMetadata{SuccessIndicator: types.TxStatusFailed}, mm.Result
	}
	return &types.ServerMetadata{ActualFee: 0, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (mm MockModule) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
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
	pdr := types.PartitionDescriptionRecord{
		NetworkIdentifier: mockNetworkID,
		SystemIdentifier:  mockTxSystemID,
		TypeIdLen:         8,
		UnitIdLen:         8 * 32,
		T2Timeout:         2500 * time.Millisecond,
	}
	// default configuration has no fee handling
	txSys, err := NewGenericTxSystem(pdr, types.ShardID{}, nil, modules, observability.Default(t))
	require.NoError(t, err)
	return txSys
}
