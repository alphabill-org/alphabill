package fc

import (
	"testing"

	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/stretchr/testify/require"
)

func TestFC_Validation(t *testing.T) {
	t.Parallel()

	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	s := state.NewEmptyState()
	partitionID := moneyPartitionID
	networkID := types.NetworkID(5)

	t.Run("new fc module validation errors", func(t *testing.T) {
		_, err := NewFeeCreditModule(0, partitionID, partitionID, s, trustBase)
		require.ErrorIs(t, err, ErrNetworkIdentifierMissing)

		_, err = NewFeeCreditModule(networkID, 0, partitionID, s, trustBase)
		require.ErrorIs(t, err, ErrPartitionIdentifierMissing)

		_, err = NewFeeCreditModule(networkID, partitionID, 0, s, trustBase)
		require.ErrorIs(t, err, ErrMoneyPartitionIdentifierMissing)

		_, err = NewFeeCreditModule(networkID, partitionID, partitionID, nil, trustBase)
		require.ErrorIs(t, err, ErrStateIsNil)

		_, err = NewFeeCreditModule(networkID, partitionID, partitionID, s, nil)
		require.ErrorIs(t, err, ErrTrustBaseIsNil)
	})

	t.Run("new fc module validation", func(t *testing.T) {
		fc, err := NewFeeCreditModule(networkID, partitionID, partitionID, s, trustBase)
		require.NoError(t, err)
		require.NotNil(t, fc)
	})

	t.Run("new fc module executors", func(t *testing.T) {
		fc, err := NewFeeCreditModule(networkID, partitionID, partitionID, s, trustBase)
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
	fcModule, err := NewFeeCreditModule(5, 10, 1, state.NewEmptyState(), trustBase)
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
