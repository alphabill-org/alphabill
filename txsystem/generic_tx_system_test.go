package txsystem

import (
	"errors"
	"math"
	"testing"

	"github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
)

const mockTxType = "mockTx-type"

func Test_NewGenericTxSystem(t *testing.T) {
	t.Run("system ID param is mandatory", func(t *testing.T) {
		txSys, err := NewGenericTxSystem(0, nil, nil, nil)
		require.Nil(t, txSys)
		require.EqualError(t, err, `system ID must be assigned`)
	})

	t.Run("success", func(t *testing.T) {
		obs := observability.Default(t)
		feeCheck := func(tx *types.TransactionOrder) error { return errors.New("FCC") }
		txSys, err := NewGenericTxSystem(
			1,
			feeCheck,
			nil,
			obs,
		)
		require.NoError(t, err)
		require.EqualValues(t, 1, txSys.systemIdentifier)
		require.NotNil(t, txSys.log)
		require.NotNil(t, txSys.checkFeeCreditBalance)
		require.EqualError(t, txSys.checkFeeCreditBalance(nil), "FCC")
	})
}

func Test_GenericTxSystem_Execute(t *testing.T) {

	createTxSystem := func(t *testing.T, modules []Module) *GenericTxSystem {
		txs, err := NewGenericTxSystem(
			1,
			func(tx *types.TransactionOrder) error { return nil }, // "all OK" fee credit validator
			modules,
			observability.Default(t),
		)
		require.NoError(t, err)
		txs.currentBlockNumber = 837644
		return txs
	}

	t.Run("tx order is validated", func(t *testing.T) {
		txSys := createTxSystem(t, nil)
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(txSys.systemIdentifier+1),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}))
		md, err := txSys.Execute(txo)
		require.ErrorIs(t, err, ErrInvalidSystemIdentifier)
		require.Nil(t, md)
	})

	t.Run("no executor for the tx type", func(t *testing.T) {
		txSys := createTxSystem(t, nil)
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(txSys.systemIdentifier),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout: txSys.currentBlockNumber + 1,
			}),
		)
		// no modules, no tx handlers
		md, err := txSys.Execute(txo)
		require.EqualError(t, err, `tx 'mockTx-type' validation error: unknown transaction type mockTx-type`)
		require.Nil(t, md)
	})

	t.Run("tx handler returns error", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(expErr)
		txSys := createTxSystem(t, []Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(txSys.systemIdentifier),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout: txSys.currentBlockNumber + 1,
			}),
		)
		md, err := txSys.Execute(txo)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, md)
	})

	t.Run("success", func(t *testing.T) {
		m := NewMockTxModule(nil)
		txSys := createTxSystem(t, []Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithSystemID(txSys.systemIdentifier),
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout: txSys.currentBlockNumber + 1,
			}),
		)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
	})
}

func Test_GenericTxSystem_validateGenericTransaction(t *testing.T) {

	// share observability between all sub-tests
	obs := observability.Default(t)

	createTxSystem := func(t *testing.T) *GenericTxSystem {
		txs, err := NewGenericTxSystem(
			1,
			func(tx *types.TransactionOrder) error { return nil }, // "all OK" fee credit validator
			nil, // test doesn't depend on modules
			obs,
		)
		require.NoError(t, err)
		txs.currentBlockNumber = 837644
		return txs
	}

	// create valid order (in the sense of basic checks performed by the generic
	// tx system) for "txs" transaction system
	createTxOrder := func(txs *GenericTxSystem) *types.TransactionOrder {
		return &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: txs.systemIdentifier,
				ClientMetadata: &types.ClientMetadata{
					Timeout: txs.currentBlockNumber + 1,
				},
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		// this (also) tests that our helper functions do create valid
		// tx system and tx order combination (other tests depend on that)
		txSys := createTxSystem(t)
		txo := createTxOrder(txSys)
		require.NoError(t, txSys.validateGenericTransaction(txo))
	})

	t.Run("system ID is checked", func(t *testing.T) {
		txSys := createTxSystem(t)
		txo := createTxOrder(txSys)
		txo.Payload.SystemID = txSys.systemIdentifier + 1
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrInvalidSystemIdentifier)
	})

	t.Run("timeout is checked", func(t *testing.T) {
		txSys := createTxSystem(t)
		txo := createTxOrder(txSys)

		txSys.currentBlockNumber = txo.Timeout()
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrTransactionExpired)
		txSys.currentBlockNumber = txo.Timeout() + 1
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrTransactionExpired)
		txSys.currentBlockNumber = math.MaxUint64
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrTransactionExpired)
	})

	t.Run("fee credit balance is checked", func(t *testing.T) {
		expErr := errors.New("nope!")
		txSys := createTxSystem(t)
		txSys.checkFeeCreditBalance = func(tx *types.TransactionOrder) error { return expErr }
		txo := createTxOrder(txSys)
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), expErr)
	})
}

type MockTxAttributes struct {
	_    struct{} `cbor:",toarray"`
	Data []byte
}

type MockModule struct {
	Result error
}

func NewMockTxModule(wantErr error) *MockModule {
	return &MockModule{Result: wantErr}
}

func (mm MockModule) mockValidateTx(tx *types.TransactionOrder, _ *MockTxAttributes, exeCtx *TxExecutionContext) (err error) {
	return mm.Result
}
func (mm MockModule) mockExecuteTx(tx *types.TransactionOrder, _ *MockTxAttributes, _ *TxExecutionContext) (*types.ServerMetadata, error) {
	if mm.Result != nil {
		return nil, mm.Result
	}
	return &types.ServerMetadata{ActualFee: 0, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (mm MockModule) TxHandlers() map[string]TxExecutor {
	return map[string]TxExecutor{
		mockTxType: NewTxHandler[MockTxAttributes](mm.mockValidateTx, mm.mockExecuteTx),
	}
}
