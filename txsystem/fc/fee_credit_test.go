package fc

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/stretchr/testify/require"
)

func TestFC_Validation(t *testing.T) {
	t.Parallel()

	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
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
		require.ErrorIs(t, err, ErrTrustBaseMissing)
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
		fcModule, err := NewFeeCreditModule(
			WithSystemIdentifier(moneySystemID),
			WithMoneySystemIdentifier(moneySystemID),
			WithState(s),
			WithTrustBase(trustBase),
		)
		require.NoError(t, err)
		require.Len(t, fcModule.TxExecutors(), 4)
		require.Contains(t, fcModule.TxExecutors(), fc.PayloadTypeAddFeeCredit)
		require.Contains(t, fcModule.TxExecutors(), fc.PayloadTypeCloseFeeCredit)
		require.Contains(t, fcModule.TxExecutors(), fc.PayloadTypeLockFeeCredit)
		require.Contains(t, fcModule.TxExecutors(), fc.PayloadTypeUnlockFeeCredit)
		require.NotContains(t, fcModule.TxExecutors(), fc.PayloadTypeTransferFeeCredit)
		require.NotContains(t, fcModule.TxExecutors(), fc.PayloadTypeReclaimFeeCredit)
	})

}
