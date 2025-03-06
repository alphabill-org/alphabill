package fc

import (
	"testing"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

var (
	moneyPartitionID types.PartitionID = 0x00000001
	recordID                           = []byte{0}
	feeProof                           = []byte{1}
)

type testData struct {
	_ struct{} `cbor:",toarray"`
}

func (t *testData) Write(hasher abhash.Hasher) { hasher.Write(t) }
func (t *testData) SummaryValueInput() uint64 {
	return 0
}
func (t *testData) Copy() types.UnitData        { return &testData{} }
func (t *testData) Owner() []byte               { return nil }
func (t *testData) GetVersion() types.ABVersion { return 0 }

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
	const feeCreditRecordUnitType = 0x0a
	pdr := types.PartitionDescriptionRecord{
		UnitIDLen: 32 * 8,
		TypeIDLen: 8,
	}
	fcrID, err := pdr.ComposeUnitID(types.ShardID{}, feeCreditRecordUnitType, func(b []byte) error { return nil })
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		s := state.NewEmptyState()
		fcr := &fc.FeeCreditRecord{
			Balance:        1,
			Counter:        10,
			MinLifetime:    2,
			OwnerPredicate: []byte{2},
		}
		require.NoError(t, s.Apply(state.AddUnit(fcrID, fcr)))
		unitData, err := parseFeeCreditRecord(&pdr, fcrID, feeCreditRecordUnitType, s)
		require.NoError(t, err)
		require.EqualValues(t, fcr, unitData)
	})

	t.Run("unit id is not fee credit type", func(t *testing.T) {
		s := state.NewEmptyState()
		unitID := []byte{1}
		require.NoError(t, s.Apply(state.AddUnit(unitID, &fc.FeeCreditRecord{})))
		unitData, err := parseFeeCreditRecord(&pdr, unitID, feeCreditRecordUnitType, s)
		require.EqualError(t, err, "invalid unit identifier: type is not fee credit record")
		require.Nil(t, unitData)
	})

	t.Run("fcr unit not found", func(t *testing.T) {
		s := state.NewEmptyState()
		unitData, err := parseFeeCreditRecord(&pdr, fcrID, feeCreditRecordUnitType, s)
		require.EqualError(t, err, "get fcr unit error: item 00000000000000000000000000000000000000000000000000000000000000000A does not exist: not found")
		require.Nil(t, unitData)
	})

	t.Run("unit data is not of type fee credit", func(t *testing.T) {
		s := state.NewEmptyState()
		fcr := &testData{}
		require.NoError(t, s.Apply(state.AddUnit(fcrID, fcr)))
		unitData, err := parseFeeCreditRecord(&pdr, fcrID, feeCreditRecordUnitType, s)
		require.EqualError(t, err, "invalid unit type: unit is not fee credit record")
		require.Nil(t, unitData)
	})
}
