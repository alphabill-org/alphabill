package fc

import (
	"testing"

	"github.com/stretchr/testify/require"

	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
)

func TestFC_Validation(t *testing.T) {
	t.Parallel()

	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	s := state.NewEmptyState()
	partitionID := moneyPartitionID
	targetPDR := moneyid.PDR()
	observe := observability.Default(t)

	t.Run("new fc module validation errors", func(t *testing.T) {
		invalidPDR := targetPDR
		invalidPDR.PartitionID = 0
		_, err := NewFeeCreditModule(invalidPDR, partitionID, s, trustBase, observe)
		require.EqualError(t, err, `invalid fee credit module configuration: invalid PDR: invalid partition identifier: 00000000`)

		_, err = NewFeeCreditModule(targetPDR, 0, s, trustBase, observe)
		require.ErrorIs(t, err, ErrMoneyPartitionIDMissing)

		_, err = NewFeeCreditModule(targetPDR, partitionID, nil, trustBase, observe)
		require.ErrorIs(t, err, ErrStateIsNil)

		_, err = NewFeeCreditModule(targetPDR, partitionID, s, nil, observe)
		require.ErrorIs(t, err, ErrTrustBaseIsNil)
	})

	t.Run("new fc module validation", func(t *testing.T) {
		fc, err := NewFeeCreditModule(targetPDR, partitionID, s, trustBase, observe)
		require.NoError(t, err)
		require.NotNil(t, fc)
	})

	t.Run("new fc module executors", func(t *testing.T) {
		fc, err := NewFeeCreditModule(targetPDR, partitionID, s, trustBase, observe)
		require.NoError(t, err)
		fcExecutors := fc.TxHandlers()
		require.Len(t, fcExecutors, 4)
		require.Contains(t, fcExecutors, fcsdk.TransactionTypeAddFeeCredit)
		require.Contains(t, fcExecutors, fcsdk.TransactionTypeCloseFeeCredit)
		require.Contains(t, fcExecutors, fcsdk.TransactionTypeLockFeeCredit)
		require.Contains(t, fcExecutors, fcsdk.TransactionTypeUnlockFeeCredit)
		require.NotContains(t, fcExecutors, fcsdk.TransactionTypeTransferFeeCredit)
		require.NotContains(t, fcExecutors, fcsdk.TransactionTypeReclaimFeeCredit)
	})

}

func TestFC_CalculateCost(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	fcModule, err := NewFeeCreditModule(moneyid.PDR(), 1, state.NewEmptyState(), trustBase, observability.Default(t))
	require.NoError(t, err)
	require.NotNil(t, fcModule)
	gas := fcModule.BuyGas(10)
	require.EqualValues(t, 10*GasUnitsPerTema, gas)
	require.EqualValues(t, 9, fcModule.CalculateCost(9*GasUnitsPerTema))
	// is rounded up
	require.EqualValues(t, 10, fcModule.CalculateCost(9*GasUnitsPerTema+GasUnitsPerTema/2))
	// is rounded down
	require.EqualValues(t, 9, fcModule.CalculateCost(9*GasUnitsPerTema+1))
	// returns always the cost of at least 1 tema
	require.EqualValues(t, 1, fcModule.CalculateCost(0))
	require.EqualValues(t, 1, fcModule.CalculateCost(100))
}
