package fc

import (
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

var (
	moneySystemID           types.SystemID = 0x00000001
	recordID                               = []byte{0}
	feeProof                               = []byte{1}
	bearer                                 = []byte{2}
	feeCreditRecordUnitType                = []byte{0xff}
)

type testData struct {
	_ struct{} `cbor:",toarray"`
}

func (t *testData) Write(hasher hash.Hash) error { return nil }
func (t *testData) SummaryValueInput() uint64 {
	return 0
}
func (t *testData) Copy() types.UnitData { return &testData{} }

// test
func TestValidateGenericFeeCreditTx(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithAttributes(&fc.AddFeeCreditAttributes{}),
		)
		require.NoError(t, ValidateGenericFeeCreditTx(tx))
	})
	t.Run("Fee credit transactions must not have fee proof", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithAttributes(&fc.AddFeeCreditAttributes{}),
		)
		tx.FeeProof = []byte{1, 2, 3}
		require.EqualError(t, ValidateGenericFeeCreditTx(tx), "fee transaction cannot contain fee authorization proof")
	})
	t.Run("Fee credit transactions must not contain FeeCreditRecordID", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithAttributes(&fc.AddFeeCreditAttributes{}),
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{1, 2, 3}}),
		)
		require.EqualError(t, ValidateGenericFeeCreditTx(tx), "fee transaction cannot contain fee credit reference")
	})
}

func TestVerifyMaxTxFeeDoesNotExceedFRCBalance(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithAttributes(&fc.AddFeeCreditAttributes{}),
			testtransaction.WithClientMetadata(
				&types.ClientMetadata{
					FeeCreditRecordID: []byte{1, 2, 3},
					MaxTransactionFee: 10,
				}),
		)
		require.NoError(t, VerifyMaxTxFeeDoesNotExceedFRCBalance(tx, 11))
	})
	t.Run("not enough max fee is more than FCR balance", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithAttributes(&fc.AddFeeCreditAttributes{}),
			testtransaction.WithClientMetadata(
				&types.ClientMetadata{
					FeeCreditRecordID: []byte{1, 2, 3},
					MaxTransactionFee: 10,
				}),
		)
		require.EqualError(t, VerifyMaxTxFeeDoesNotExceedFRCBalance(tx, 9),
			"max fee cannot exceed fee credit record balance: tx.maxFee=10 fcr.Balance=9")
	})
}

func TestValidateCloseFC(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		attr := &fc.CloseFeeCreditAttributes{
			Amount:       10,
			TargetUnitID: []byte{1, 2, 3},
		}
		fcr := &fc.FeeCreditRecord{
			Balance: 10,
			Locked:  0,
		}
		require.NoError(t, ValidateCloseFC(attr, fcr))
	})
	t.Run("FRC balance must match close attributes", func(t *testing.T) {
		attr := &fc.CloseFeeCreditAttributes{
			Amount:       10,
			TargetUnitID: []byte{1, 2, 3},
		}
		fcr := &fc.FeeCreditRecord{
			Balance: 11,
			Locked:  0,
		}
		require.EqualError(t, ValidateCloseFC(attr, fcr), "invalid amount: amount=10 fcr.Balance=11")
	})
	t.Run("target unit id must not be empty", func(t *testing.T) {
		attr := &fc.CloseFeeCreditAttributes{
			Amount:       10,
			TargetUnitID: nil,
		}
		fcr := &fc.FeeCreditRecord{
			Balance: 10,
			Locked:  0,
		}
		require.EqualError(t, ValidateCloseFC(attr, fcr), "TargetUnitID is empty")
	})
	t.Run("unit is locked", func(t *testing.T) {
		attr := &fc.CloseFeeCreditAttributes{
			Amount:       10,
			TargetUnitID: []byte{1, 2, 3},
		}
		fcr := &fc.FeeCreditRecord{
			Balance: 10,
			Locked:  1,
		}
		require.EqualError(t, ValidateCloseFC(attr, fcr), "fee credit record is locked")
	})
}

func Test_parseFeeCreditRecord(t *testing.T) {
	feeCreditRecordUnitType = []byte{0x0a}

	t.Run("ok", func(t *testing.T) {
		s := state.NewEmptyState()
		fcr := &fc.FeeCreditRecord{
			Balance: 1,
			Counter: 10,
			Timeout: 2,
		}
		unitPart := []byte{1}
		fcrID := types.NewUnitID(33, nil, unitPart, feeCreditRecordUnitType)
		require.NoError(t, s.Apply(state.AddUnit(fcrID, templates.AlwaysTrueBytes(), fcr)))
		unitData, bearer, err := parseFeeCreditRecord(fcrID, feeCreditRecordUnitType, s)
		require.NoError(t, err)
		require.EqualValues(t, bearer, templates.AlwaysTrueBytes())
		require.EqualValues(t, fcr, unitData)
	})
	t.Run("unit id is not fee credit type", func(t *testing.T) {
		s := state.NewEmptyState()
		unitID := []byte{1}
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{})))
		unitData, bearer, err := parseFeeCreditRecord(unitID, feeCreditRecordUnitType, s)
		require.EqualError(t, err, "invalid unit identifier: type is not fee credit record")
		require.Nil(t, bearer)
		require.Nil(t, unitData)
	})
	t.Run("fcr unit not found", func(t *testing.T) {
		s := state.NewEmptyState()
		unitPart := []byte{1}
		fcrID := types.NewUnitID(33, nil, unitPart, feeCreditRecordUnitType)
		unitData, bearer, err := parseFeeCreditRecord(fcrID, feeCreditRecordUnitType, s)
		require.EqualError(t, err, "get fcr unit error: item 00000000000000000000000000000000000000000000000000000000000000010A does not exist: not found")
		require.Nil(t, bearer)
		require.Nil(t, unitData)
	})
	t.Run("unit data is not of type fee credit", func(t *testing.T) {
		s := state.NewEmptyState()
		fcr := &testData{}
		unitPart := []byte{1}
		fcrID := types.NewUnitID(33, nil, unitPart, feeCreditRecordUnitType)
		require.NoError(t, s.Apply(state.AddUnit(fcrID, templates.AlwaysTrueBytes(), fcr)))
		unitData, bearer, err := parseFeeCreditRecord(fcrID, feeCreditRecordUnitType, s)
		require.EqualError(t, err, "invalid unit type: unit is not fee credit record")
		require.Nil(t, bearer)
		require.Nil(t, unitData)
	})
}
