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
	networkID := types.NetworkID(5)
	systemID := types.SystemID(5)
	feeCreditRecordUnitType := []byte{1}
	adminOwnerPredicate := predtempl.NewP2pkh256BytesFromKey(pubKey)

	t.Run("missing network id", func(t *testing.T) {
		m, err := NewFeeCreditModule(0, systemID, stateTree, feeCreditRecordUnitType, adminOwnerPredicate)
		require.Nil(t, m)
		require.ErrorIs(t, err, ErrMissingSystemIdentifier)
	})

	t.Run("missing system id", func(t *testing.T) {
		m, err := NewFeeCreditModule(networkID, 0, stateTree, feeCreditRecordUnitType, adminOwnerPredicate)
		require.Nil(t, m)
		require.ErrorIs(t, err, ErrMissingSystemIdentifier)
	})

	t.Run("state is nil", func(t *testing.T) {
		m, err := NewFeeCreditModule(networkID, systemID, nil, feeCreditRecordUnitType, adminOwnerPredicate)
		require.Nil(t, m)
		require.ErrorIs(t, err, ErrStateIsNil)
	})

	t.Run("fee credit record unit type is nil", func(t *testing.T) {
		m, err := NewFeeCreditModule(networkID, systemID, stateTree, nil, adminOwnerPredicate)
		require.Nil(t, m)
		require.ErrorIs(t, err, ErrMissingFeeCreditRecordUnitType)
	})

	t.Run("admin owner predicate is nil", func(t *testing.T) {
		m, err := NewFeeCreditModule(networkID, systemID, stateTree, feeCreditRecordUnitType, nil)
		require.Nil(t, m)
		require.ErrorIs(t, err, ErrMissingAdminOwnerPredicate)
	})

	t.Run("ok", func(t *testing.T) {
		m, err := NewFeeCreditModule(networkID, systemID, stateTree, feeCreditRecordUnitType, adminOwnerPredicate)
		require.NoError(t, err)
		require.NotNil(t, m)
		require.NotNil(t, m.execPredicate, "execPredicate should not be nil")
		require.NotNil(t, m.feeBalanceValidator, "feeBalanceValidator should not be nil")
	})
}
