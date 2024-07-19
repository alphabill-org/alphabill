package permissioned

import (
	"testing"

	predtempl "github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/stretchr/testify/require"
)

func TestNewFeeCreditModule(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	stateTree := state.NewEmptyState()
	systemID := types.SystemID(5)
	feeCreditRecordUnitType := []byte{1}
	adminOwnerCondition := predtempl.NewP2pkh256BytesFromKey(pubKey)

	t.Run("missing system id", func(t *testing.T) {
		m, err := NewFeeCreditModule(
			WithSystemIdentifier(0),
			WithState(stateTree),
			WithFeeCreditRecordUnitType(feeCreditRecordUnitType),
			WithAdminOwnerCondition(adminOwnerCondition),
		)
		require.Nil(t, m)
		require.ErrorIs(t, err, ErrMissingSystemIdentifier)
	})

	t.Run("state is nil", func(t *testing.T) {
		m, err := NewFeeCreditModule(
			WithSystemIdentifier(systemID),
			WithState(nil),
			WithFeeCreditRecordUnitType(feeCreditRecordUnitType),
			WithAdminOwnerCondition(adminOwnerCondition),
		)
		require.Nil(t, m)
		require.ErrorIs(t, err, ErrStateIsNil)
	})

	t.Run("fee credit record unit type is nil", func(t *testing.T) {
		m, err := NewFeeCreditModule(
			WithSystemIdentifier(systemID),
			WithState(stateTree),
			WithFeeCreditRecordUnitType(nil),
			WithAdminOwnerCondition(adminOwnerCondition),
		)
		require.Nil(t, m)
		require.ErrorIs(t, err, ErrMissingFeeCreditRecordUnitType)
	})

	t.Run("admin owner condition is nil", func(t *testing.T) {
		m, err := NewFeeCreditModule(
			WithSystemIdentifier(systemID),
			WithState(stateTree),
			WithFeeCreditRecordUnitType(feeCreditRecordUnitType),
			WithAdminOwnerCondition(nil),
		)
		require.Nil(t, m)
		require.ErrorIs(t, err, ErrMissingAdminOwnerCondition)
	})

	t.Run("ok", func(t *testing.T) {
		m, err := NewFeeCreditModule(
			WithSystemIdentifier(systemID),
			WithState(stateTree),
			WithFeeCreditRecordUnitType(feeCreditRecordUnitType),
			WithAdminOwnerCondition(adminOwnerCondition),
		)
		require.NoError(t, err)
		require.NotNil(t, m)

		require.NotNil(t, m.execPredicate, "execPredicate should not be nil")
		require.NotNil(t, m.feeBalanceValidator, "feeBalanceValidator should not be nil")
	})
}
