package txsystem

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/types"
)

func Test_NewGenericTxSystem(t *testing.T) {
	t.Run("system ID param is mandatory", func(t *testing.T) {
		txSys, err := NewGenericTxSystem(nil, nil, nil)
		require.Nil(t, txSys)
		require.EqualError(t, err, `system ID must be assigned`)
	})

	t.Run("success", func(t *testing.T) {
		logr := logger.New(t)
		feeCheck := func(tx *types.TransactionOrder) error { return errors.New("FCC") }
		txSys, err := NewGenericTxSystem(
			logr,
			feeCheck,
			nil,
			WithSystemIdentifier(1),
		)
		require.NoError(t, err)
		require.EqualValues(t, 1, txSys.systemIdentifier)
		require.Equal(t, logr, txSys.log)
		require.NotNil(t, txSys.checkFeeCreditBalance)
		require.EqualError(t, txSys.checkFeeCreditBalance(nil), "FCC")
	})
}

func Test_GenericTxSystem_Execute(t *testing.T) {

	createTxSystem := func(t *testing.T, modules []Module) *GenericTxSystem {
		txs, err := NewGenericTxSystem(
			logger.New(t),
			func(tx *types.TransactionOrder) error { return nil }, // "all OK" fee credit validator
			modules,
			WithSystemIdentifier(1),
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
				Type:     "tx-type",
				ClientMetadata: &types.ClientMetadata{
					Timeout: txs.currentBlockNumber + 1,
				},
			},
		}
	}

	t.Run("tx order is validated", func(t *testing.T) {
		txSys := createTxSystem(t, nil)
		txo := createTxOrder(txSys)
		// make a change that should cause tx validation fo fail, ie we check that
		// before executing the tx the validateGenericTransaction method is called
		txo.Payload.SystemID = txSys.systemIdentifier + 1
		md, err := txSys.Execute(txo)
		require.ErrorIs(t, err, ErrInvalidSystemIdentifier)
		require.Nil(t, md)
	})

	t.Run("no executor for the tx type", func(t *testing.T) {
		txSys := createTxSystem(t, nil) // no modules, no tx handlers
		txo := createTxOrder(txSys)
		md, err := txSys.Execute(txo)
		require.EqualError(t, err, `unknown transaction type tx-type`)
		require.Nil(t, md)
	})

	t.Run("tx handler returns error", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := mockModule{
			getExecutors: func() map[string]TxExecutor {
				return map[string]TxExecutor{
					"tx-type": ExecuteFunc(func(tx *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error) {
						return nil, expErr
					}),
				}
			},
		}
		txSys := createTxSystem(t, []Module{m})
		txo := createTxOrder(txSys)
		md, err := txSys.Execute(txo)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, md)
	})

	t.Run("success", func(t *testing.T) {
		m := mockModule{getExecutors: func() map[string]TxExecutor {
			return map[string]TxExecutor{
				"tx-type": ExecuteFunc(func(tx *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error) {
					return &types.ServerMetadata{SuccessIndicator: types.TxStatusSuccessful}, nil
				}),
			}
		},
		}
		txSys := createTxSystem(t, []Module{m})
		txo := createTxOrder(txSys)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
	})
}

func Test_GenericTxSystem_validateGenericTransaction(t *testing.T) {

	// these tests shouldn't need logger but create one and share it for all sub-tests
	logr := logger.New(t)

	createTxSystem := func(t *testing.T) *GenericTxSystem {
		txs, err := NewGenericTxSystem(
			logr,
			func(tx *types.TransactionOrder) error { return nil }, // "all OK" fee credit validator
			nil, // test doesn't depend on modules
			WithSystemIdentifier(1),
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

type mockModule struct {
	getExecutors func() map[string]TxExecutor
}

func (mm mockModule) TxExecutors() map[string]TxExecutor {
	return mm.getExecutors()
}
