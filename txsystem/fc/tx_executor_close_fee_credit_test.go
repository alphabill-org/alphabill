package fc

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/stretchr/testify/require"
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

	existingFCR := &fc.FeeCreditRecord{Locked: 1}
	require.NoError(t, s.Apply(state.AddUnit(tx.UnitID(), nil, existingFCR)))

	sm, err := execFn(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
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
	existingFCR := &fc.FeeCreditRecord{Balance: 50}
	require.NoError(t, s.Apply(state.AddUnit(tx.UnitID(), nil, existingFCR)))

	// execute closeFC transaction
	sm, err := execFn(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify closeFC updated the FCR.Backlink
	fcrUnit, err := s.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, tx.Hash(crypto.SHA256), fcr.Backlink)
}
