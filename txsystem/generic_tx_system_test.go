package txsystem

import (
	"crypto"
	"errors"
	"math"
	"testing"
	"time"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

const mockTxType uint16 = 1
const mockSplitTxType uint16 = 2
const mockFeeTxType uint16 = 3
const mockNetworkID types.NetworkID = 5     // same as txsystem/testutils/transaction#defaultNetworkID
const mockPartitionID types.PartitionID = 1 // same as txsystem/testutils/transaction#defaultPartitionID

type MockData struct {
	_              struct{} `cbor:",toarray"`
	Value          uint64
	OwnerPredicate []byte
}

func (t *MockData) Write(hasher abhash.Hasher) {
	hasher.Write(t)
}

func (t *MockData) SummaryValueInput() uint64 {
	return t.Value
}
func (t *MockData) Copy() types.UnitData {
	return &MockData{Value: t.Value}
}
func (t *MockData) Owner() []byte {
	return t.OwnerPredicate
}

func (t *MockData) GetVersion() types.ABVersion { return 0 }

func Test_NewGenericTxSystem(t *testing.T) {
	validPDR := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   mockNetworkID,
		PartitionID: mockPartitionID,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   2500 * time.Millisecond,
	}
	require.NoError(t, validPDR.IsValid())

	t.Run("partition ID param is mandatory", func(t *testing.T) {
		pdr := validPDR
		pdr.PartitionID = 0
		txSys, err := NewGenericTxSystem(pdr, types.ShardID{}, nil, nil, nil, nil)
		require.Nil(t, txSys)
		require.EqualError(t, err, `invalid Partition Description: invalid partition identifier: 00000000`)
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
		require.EqualValues(t, mockPartitionID, txSys.pdr.PartitionID)
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
			transaction.WithPartitionID(mockPartitionID+1),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}))
		txr, err := txSys.Execute(txo)
		require.ErrorIs(t, err, ErrInvalidPartitionID)
		require.Nil(t, txr)
	})

	t.Run("no executor for the tx type", func(t *testing.T) {
		txSys := NewTestGenericTxSystem(t, nil)
		txo := transaction.NewTransactionOrder(t,
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		// no modules, no tx handlers
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
		require.EqualValues(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	})

	t.Run("tx validate returns error", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(nil)
		m.ValidateError = expErr
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
		require.EqualValues(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	})

	t.Run("tx validate returns out of gas", func(t *testing.T) {
		expErr := types.ErrOutOfGas
		m := NewMockTxModule(nil)
		m.ValidateError = expErr
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
		require.EqualValues(t, types.TxErrOutOfGas, txr.ServerMetadata.SuccessIndicator)
	})

	t.Run("tx execute returns error", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(expErr)
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
		require.EqualValues(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	})

	t.Run("locked unit - unlock fails", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(expErr)
		unitID := test.RandomBytes(33)
		fcrID := test.RandomBytes(33)
		txSys := NewTestGenericTxSystem(t,
			[]txtypes.Module{m},
			withStateUnit(fcrID, &fcsdk.FeeCreditRecord{Balance: 10, OwnerPredicate: templates.AlwaysTrueBytes()}, nil),
			withStateUnit(unitID, &MockData{
				Value:          1,
				OwnerPredicate: templates.AlwaysTrueBytes()},
				newMockLockTx(t,
					transaction.WithPartitionID(mockPartitionID),
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
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
		)
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
		require.EqualValues(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	})

	t.Run("locked unit - unlocked, but execution fails", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(expErr)
		unitID := test.RandomBytes(33)
		fcrID := test.RandomBytes(33)
		txSys := NewTestGenericTxSystem(t,
			[]txtypes.Module{m},
			withStateUnit(fcrID, &fcsdk.FeeCreditRecord{Balance: 10, OwnerPredicate: templates.AlwaysTrueBytes()}, nil),
			withStateUnit(unitID, &MockData{Value: 1, OwnerPredicate: templates.AlwaysTrueBytes()},
				newMockLockTx(t,
					transaction.WithPartitionID(mockPartitionID),
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
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				FeeCreditRecordID: fcrID,
				MaxTransactionFee: 10,
			}),
			transaction.WithStateUnlock([]byte{byte(StateUnlockExecute)}),
		)
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
		require.EqualValues(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	})

	t.Run("lock fails - validate fails", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := NewMockTxModule(nil)
		m.ValidateError = expErr
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithPartitionID(mockPartitionID),
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
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
		require.EqualValues(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	})

	t.Run("lock fails - state lock invalid", func(t *testing.T) {
		m := NewMockTxModule(nil)
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m})
		txo := transaction.NewTransactionOrder(t,
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1,
			}),
			transaction.WithStateLock(&types.StateLock{}),
		)
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
		require.EqualValues(t, types.TxStatusFailed, txr.ServerMetadata.SuccessIndicator)
	})

	t.Run("lock success", func(t *testing.T) {
		m := NewMockTxModule(nil)
		unitID := test.RandomBytes(33)
		fcrID := test.RandomBytes(33)
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m},
			withStateUnit(unitID, &MockData{Value: 1, OwnerPredicate: templates.AlwaysTrueBytes()}, nil),
			withStateUnit(fcrID, &fcsdk.FeeCreditRecord{Balance: 10, OwnerPredicate: templates.AlwaysTrueBytes()}, nil))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithUnitID(unitID),
			transaction.WithPartitionID(mockPartitionID),
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
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
	})

	t.Run("success", func(t *testing.T) {
		m := NewMockTxModule(nil)
		fcrID := test.RandomBytes(33)
		txSys := NewTestGenericTxSystem(t, []txtypes.Module{m},
			withStateUnit(fcrID, &fcsdk.FeeCreditRecord{Balance: 10, OwnerPredicate: templates.AlwaysTrueBytes()}, nil))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				FeeCreditRecordID: fcrID,
				MaxTransactionFee: 1,
			}),
		)
		txr, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, txr.ServerMetadata)
	})
}

func Test_GenericTxSystem_Execute_FeeTransactions(t *testing.T) {
	t.Run("fee tx is discarded if tx contains StateLock", func(t *testing.T) {
		txSys := createTxSystemWithFees(t)

		// create fee tx with StateLock
		txo := transaction.NewTransactionOrder(t,
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockFeeTxType),
			transaction.WithAttributes(mockFeeTxAttributes{}),
			transaction.WithAuthProof(mockFeeTxAuthProof{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1000,
			}),
			transaction.WithStateLock(&types.StateLock{}),
		)

		// execute fee tx and verify fee tx is discarded due to having StateLock
		tr, err := txSys.Execute(txo)
		require.ErrorContains(t, err, "error fc transaction contains state lock")
		require.Nil(t, tr)
	})

	t.Run("test fee tx is discarded if target unit is locked with non-nop tx", func(t *testing.T) {
		txSys := createTxSystemWithFees(t)

		// create txOnHold (non-nop tx)
		txOnHold := transaction.NewTransactionOrder(t,
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockFeeTxType),
			transaction.WithAttributes(mockFeeTxAttributes{}),
			transaction.WithAuthProof(mockFeeTxAuthProof{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1000,
			}),
			transaction.WithStateLock(&types.StateLock{}),
		)
		txOnHoldBytes, err := types.Cbor.Marshal(txOnHold)
		require.NoError(t, err)

		// create FCR unit with txOnHold
		fcrID := test.RandomBytes(33)
		err = txSys.state.Apply(state.AddUnitWithLock(fcrID, nil, txOnHoldBytes))
		require.NoError(t, err)

		// create fee tx
		tx := transaction.NewTransactionOrder(t,
			transaction.WithUnitID(fcrID),
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(mockFeeTxType),
			transaction.WithAttributes(mockFeeTxAttributes{}),
			transaction.WithAuthProof(mockFeeTxAuthProof{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1000,
			}),
		)

		// execute fee tx and verify fee tx is discarded due to being locked with non-nop tx
		tr, err := txSys.Execute(tx)
		require.ErrorContains(t, err, "target unit is locked with a non-trivial (non-NOP) transaction")
		require.Nil(t, tr)
	})

	t.Run("test FCR can be locked and unlocked with NOP transaction ", func(t *testing.T) {
		txSys := createTxSystemWithFees(t)

		// create FCR
		fcrID := test.RandomBytes(33)
		err := txSys.state.Apply(state.AddUnit(fcrID, &fcsdk.FeeCreditRecord{
			Balance: 100,
			Counter: 0,
		}))
		require.NoError(t, err)

		// create NOP tx with StateLock
		tx := transaction.NewTransactionOrder(t,
			transaction.WithUnitID(fcrID),
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(nop.TransactionTypeNOP),
			transaction.WithAttributes(nop.Attributes{}),
			transaction.WithAuthProof(nop.AuthProof{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1000,
			}),
			transaction.WithStateLock(&types.StateLock{
				ExecutionPredicate: templates.AlwaysTrueBytes(),
				RollbackPredicate:  templates.AlwaysTrueBytes(),
			}),
		)

		// execute NOP tx and verify FCR is locked
		tr, err := txSys.Execute(tx)
		require.NoError(t, err)
		require.NotNil(t, tr)
		u, err := txSys.state.GetUnit(fcrID, false)
		require.NoError(t, err)
		unitV1, err := state.ToUnitV1(u)
		require.NoError(t, err)
		expectedTxBytes, err := cbor.Marshal(tx)
		require.NoError(t, err)
		require.Equal(t, expectedTxBytes, unitV1.StateLockTx())

		// create NOP tx with StateUnlock
		tx = transaction.NewTransactionOrder(t,
			transaction.WithUnitID(fcrID),
			transaction.WithPartitionID(mockPartitionID),
			transaction.WithTransactionType(nop.TransactionTypeNOP),
			transaction.WithAttributes(nop.Attributes{}),
			transaction.WithAuthProof(nop.AuthProof{}),
			transaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				MaxTransactionFee: 1000,
			}),
			transaction.WithStateUnlock([]byte{byte(StateUnlockExecute)}),
		)

		// execute NOP tx and verify FCR is unlocked
		tr, err = txSys.Execute(tx)
		require.NoError(t, err)
		require.NotNil(t, tr)
		u, err = txSys.state.GetUnit(fcrID, false)
		require.NoError(t, err)
		unitV1, err = state.ToUnitV1(u)
		require.NoError(t, err)
		expectedTxBytes, err = cbor.Marshal(tx)
		require.NoError(t, err)
		require.False(t, unitV1.IsStateLocked())
	})
}

func createTxSystemWithFees(t *testing.T) *GenericTxSystem {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	pdr := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   mockNetworkID,
		PartitionID: mockPartitionID,
		TypeIDLen:   8,
		UnitIDLen:   8 * 32,
		T2Timeout:   2500 * time.Millisecond,
	}
	obs := observability.Default(t)
	feeModule := newMockFeeModule()
	m := NewMockTxModule(nil)
	txSys, err := NewGenericTxSystem(pdr, types.ShardID{}, trustBase, []txtypes.Module{m}, obs, WithFeeCredits(feeModule))
	require.NoError(t, err)
	return txSys
}

func Test_GenericTxSystem_validateGenericTransaction(t *testing.T) {
	// create valid order (in the sense of basic checks performed by the generic
	// tx system) for "txs" transaction system
	createTxOrder := func(txs *GenericTxSystem) *types.TransactionOrder {
		return &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				NetworkID:   txs.pdr.NetworkID,
				PartitionID: txs.pdr.PartitionID,
				UnitID:      make(types.UnitID, 33),
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

	t.Run("partition ID is checked", func(t *testing.T) {
		txSys := NewTestGenericTxSystem(t, nil)
		txo := createTxOrder(txSys)
		txo.PartitionID = txSys.pdr.PartitionID + 1
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrInvalidPartitionID)
	})

	t.Run("timeout is checked", func(t *testing.T) {
		txSys := NewTestGenericTxSystem(t, nil)
		txo := createTxOrder(txSys)

		txSys.currentRoundNumber = txo.Timeout() - 1
		require.NoError(t, txSys.validateGenericTransaction(txo), "tx.Timeout > currentRoundNumber should be valid")

		txSys.currentRoundNumber = txo.Timeout()
		require.NoError(t, txSys.validateGenericTransaction(txo), "tx.Timeout == currentRoundNumber should be valid")

		txSys.currentRoundNumber = txo.Timeout() + 1
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrTransactionExpired)

		txSys.currentRoundNumber = math.MaxUint64
		require.ErrorIs(t, txSys.validateGenericTransaction(txo), ErrTransactionExpired)
	})
}

func Test_GenericTxSystem_ExecutedTransactionsBuffer(t *testing.T) {
	t.Run("same transaction cannot be executed twice", func(t *testing.T) {
		txSystem := NewTestGenericTxSystem(t, nil)
		txo := transaction.NewTransactionOrder(t)
		_, err := txSystem.Execute(txo)
		require.NoError(t, err)

		// execute same tx again
		_, err = txSystem.Execute(txo)
		require.ErrorContains(t, err, "transaction already executed")
	})
	t.Run("executing multiple different transactions ok", func(t *testing.T) {
		txSystem := NewTestGenericTxSystem(t, nil)

		tx1 := transaction.NewTransactionOrder(t, transaction.WithClientMetadata(&types.ClientMetadata{Timeout: 1}))
		tx1Hash, err := tx1.Hash(crypto.SHA256)
		require.NoError(t, err)

		tx2 := transaction.NewTransactionOrder(t, transaction.WithClientMetadata(&types.ClientMetadata{Timeout: 2}))
		tx2Hash, err := tx2.Hash(crypto.SHA256)
		require.NoError(t, err)

		_, err = txSystem.Execute(tx1)
		require.NoError(t, err)
		_, err = txSystem.Execute(tx2)
		require.NoError(t, err)

		tx1Timeout, _ := txSystem.etBuffer.Get(string(tx1Hash))
		tx2Timeout, _ := txSystem.etBuffer.Get(string(tx2Hash))
		require.Equal(t, uint64(1), tx1Timeout)
		require.Equal(t, uint64(2), tx2Timeout)
	})
	t.Run("expired transactions are deleted on EndBlock", func(t *testing.T) {
		txSystem := NewTestGenericTxSystem(t, nil)
		tx1 := transaction.NewTransactionOrder(t, transaction.WithClientMetadata(&types.ClientMetadata{
			Timeout: 0,
		}))
		tx1Hash, err := tx1.Hash(crypto.SHA256)
		require.NoError(t, err)

		tx2 := transaction.NewTransactionOrder(t, transaction.WithClientMetadata(&types.ClientMetadata{
			Timeout: 1,
		}))
		tx2Hash, err := tx2.Hash(crypto.SHA256)
		require.NoError(t, err)

		_, err = txSystem.Execute(tx1)
		require.NoError(t, err)

		_, err = txSystem.Execute(tx2)
		require.NoError(t, err)

		_, err = txSystem.EndBlock()
		require.NoError(t, err)

		timeout, f := txSystem.etBuffer.Get(string(tx1Hash))
		require.False(t, f)
		require.Zero(t, timeout)

		timeout, f = txSystem.etBuffer.Get(string(tx2Hash))
		require.True(t, f)
		require.Equal(t, uint64(1), timeout)
	})
	t.Run("revert rolls back pending transactions", func(t *testing.T) {
		txSystem := NewTestGenericTxSystem(t, nil)
		tx := transaction.NewTransactionOrder(t, transaction.WithClientMetadata(&types.ClientMetadata{
			Timeout: 10,
		}))
		txHash, err := tx.Hash(crypto.SHA256)
		require.NoError(t, err)

		_, err = txSystem.Execute(tx)
		require.NoError(t, err)

		txSystem.Revert()

		timeout, f := txSystem.etBuffer.Get(string(txHash))
		require.False(t, f)
		require.Zero(t, timeout)
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

type MockSplitTxAttributes struct {
	_           struct{} `cbor:",toarray"`
	Value       uint64
	TargetUnits []types.UnitID
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

func (mm MockModule) mockValidateSplitTx(tx *types.TransactionOrder, _ *MockSplitTxAttributes, _ *MockTxAuthProof, _ txtypes.ExecutionContext) (err error) {
	return mm.ValidateError
}

func (mm MockModule) mockExecuteSplitTx(tx *types.TransactionOrder, _ *MockSplitTxAttributes, _ *MockTxAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	if mm.Result != nil {
		return &types.ServerMetadata{SuccessIndicator: types.TxStatusFailed}, mm.Result
	}
	return &types.ServerMetadata{ActualFee: 0, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (mm MockModule) mockSplitTargetUnits(tx *types.TransactionOrder, attr *MockSplitTxAttributes, _ *MockTxAuthProof, _ txtypes.ExecutionContext) ([]types.UnitID, error) {
	return attr.TargetUnits, nil
}

func (mm MockModule) mockValidateNopTx(tx *types.TransactionOrder, _ *nop.Attributes, _ *nop.AuthProof, _ txtypes.ExecutionContext) (err error) {
	return nil
}

func (mm MockModule) mockExecuteNopTx(tx *types.TransactionOrder, _ *nop.Attributes, _ *nop.AuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	return &types.ServerMetadata{ActualFee: 0, SuccessIndicator: types.TxStatusSuccessful, TargetUnits: []types.UnitID{tx.UnitID}}, nil
}

func (mm MockModule) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		mockTxType:             txtypes.NewTxHandler[MockTxAttributes, MockTxAuthProof](mm.mockValidateTx, mm.mockExecuteTx),
		mockSplitTxType:        txtypes.NewTxHandler[MockSplitTxAttributes, MockTxAuthProof](mm.mockValidateSplitTx, mm.mockExecuteSplitTx, txtypes.WithTargetUnitsFn(mm.mockSplitTargetUnits)),
		nop.TransactionTypeNOP: txtypes.NewTxHandler[nop.Attributes, nop.AuthProof](mm.mockValidateNopTx, mm.mockExecuteNopTx),
	}
}

type txSystemTestOption func(m *GenericTxSystem) error

func withStateUnit(unitID []byte, data types.UnitData, lock []byte) txSystemTestOption {
	return func(m *GenericTxSystem) error {
		return m.state.Apply(state.AddUnitWithLock(unitID, data, lock))
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
		Version:     1,
		NetworkID:   mockNetworkID,
		PartitionID: mockPartitionID,
		TypeIDLen:   8,
		UnitIDLen:   8 * 32,
		T2Timeout:   2500 * time.Millisecond,
	}
	// default configuration has no fee handling
	txSys, err := NewGenericTxSystem(pdr, types.ShardID{}, nil, modules, observability.Default(t))
	require.NoError(t, err)
	return txSys
}

var _ txtypes.Module = (*mockFeeModule)(nil)

type mockFeeModule struct{}

type mockFeeTxAttributes struct {
	_     struct{} `cbor:",toarray"`
	Value uint64
}

type mockFeeTxAuthProof struct {
	_          struct{} `cbor:",toarray"`
	OwnerProof []byte
}

func newMockFeeModule() *mockFeeModule {
	return &mockFeeModule{}
}

func (f *mockFeeModule) CalculateCost(_ uint64) uint64 {
	return 0
}

func (f *mockFeeModule) BuyGas(_ uint64) uint64 {
	return math.MaxUint64
}

func (f *mockFeeModule) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		mockFeeTxType: txtypes.NewTxHandler[mockFeeTxAttributes, mockFeeTxAuthProof](f.validateFeeTx, f.executeFeeTx),
	}
}

func (f *mockFeeModule) IsCredible(_ txtypes.ExecutionContext, _ *types.TransactionOrder) error {
	return nil
}

func (f *mockFeeModule) IsFeeCreditTx(tx *types.TransactionOrder) bool {
	return tx.Type == mockFeeTxType
}

func (f *mockFeeModule) IsPermissionedMode() bool {
	return false
}

func (f *mockFeeModule) IsFeelessMode() bool {
	return true
}

func (f *mockFeeModule) validateFeeTx(tx *types.TransactionOrder, _ *mockFeeTxAttributes, _ *mockFeeTxAuthProof, _ txtypes.ExecutionContext) (err error) {
	return nil
}

func (f *mockFeeModule) executeFeeTx(tx *types.TransactionOrder, _ *mockFeeTxAttributes, _ *mockFeeTxAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	return &types.ServerMetadata{ActualFee: 0, SuccessIndicator: types.TxStatusSuccessful}, nil
}
