package fc

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/state"
	testsig "github.com/alphabill-org/alphabill/validator/internal/testutils/sig"
	testfc "github.com/alphabill-org/alphabill/validator/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/validator/internal/txsystem/fc/unit"
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

func TestCloseFC_UpdatesBacklink(t *testing.T) {
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

	// create existing fee credit record for closeFC
	attr := testfc.NewCloseFCAttr()
	tx := testfc.NewCloseFC(t, attr)
	existingFCR := &unit.FeeCreditRecord{Balance: 50}
	require.NoError(t, s.Apply(state.AddUnit(tx.UnitID(), nil, existingFCR)))

	// execute closeFC transaction
	sm, err := execFn(tx, attr, 10)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify closeFC updated the FCR.Backlink
	fcrUnit, err := s.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := fcrUnit.Data().(*unit.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, tx.Hash(crypto.SHA256), fcr.Backlink)
}
