package fc

import (
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/state"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
)

func TestCloseFC_CannotCloseLockedCredit(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
	s := state.NewEmptyState()
	feeCreditModule, err := NewFeeCreditModule(
		WithSystemIdentifier(moneySystemID),
		WithMoneySystemIdentifier(moneySystemID),
		WithState(s),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)
	execFn := handleCloseFeeCreditTx(feeCreditModule)

	attr := testfc.NewCloseFCAttr()
	tx := testfc.NewCloseFC(t, attr)

	existingFCR := &unit.FeeCreditRecord{Locked: 1}
	require.NoError(t, s.Apply(state.AddUnit(tx.UnitID(), nil, existingFCR)))

	sm, err := execFn(tx, attr, 10)
	require.ErrorContains(t, err, "fee credit record is locked")
	require.Nil(t, sm)
}
