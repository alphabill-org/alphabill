package fc

import (
	"testing"

	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"

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

	t.Run("new fc module validation errors", func(t *testing.T) {
		fcModule, err := NewFeeCreditModule()
		require.Nil(t, fcModule)
		require.ErrorIs(t, err, ErrSystemIdentifierMissing)

		_, err = NewFeeCreditModule(WithSystemIdentifier(moneySystemID))
		require.ErrorIs(t, err, ErrMoneySystemIdentifierMissing)

		_, err = NewFeeCreditModule(WithSystemIdentifier(moneySystemID),
			WithMoneySystemIdentifier(moneySystemID))
		require.ErrorIs(t, err, ErrStateIsNil)

		_, err = NewFeeCreditModule(WithSystemIdentifier(moneySystemID),
			WithMoneySystemIdentifier(moneySystemID),
			WithState(s))
		require.ErrorIs(t, err, ErrTrustBaseIsNil)
	})

	t.Run("new fc module validation", func(t *testing.T) {
		fc, err := NewFeeCreditModule(
			WithSystemIdentifier(moneySystemID),
			WithMoneySystemIdentifier(moneySystemID),
			WithState(s),
			WithTrustBase(trustBase),
		)
		require.NoError(t, err)
		require.NotNil(t, fc)
	})

	t.Run("new fc module executors", func(t *testing.T) {
		fc, err := NewFeeCreditModule(
			WithSystemIdentifier(moneySystemID),
			WithMoneySystemIdentifier(moneySystemID),
			WithState(s),
			WithTrustBase(trustBase),
		)
		require.NoError(t, err)
		fcExecutors := fc.TxHandlers()
		require.Len(t, fcExecutors, 4)
		require.Contains(t, fcExecutors, fcsdk.PayloadTypeAddFeeCredit)
		require.Contains(t, fcExecutors, fcsdk.PayloadTypeCloseFeeCredit)
		require.Contains(t, fcExecutors, fcsdk.PayloadTypeLockFeeCredit)
		require.Contains(t, fcExecutors, fcsdk.PayloadTypeUnlockFeeCredit)
		require.NotContains(t, fcExecutors, fcsdk.PayloadTypeTransferFeeCredit)
		require.NotContains(t, fcExecutors, fcsdk.PayloadTypeReclaimFeeCredit)
	})

}
