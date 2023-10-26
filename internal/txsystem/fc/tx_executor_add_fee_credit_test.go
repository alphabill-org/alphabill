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

func TestAddFC_AddNewFeeCredit(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
	feeCreditModule, err := NewFeeCreditModule(
		WithSystemIdentifier(moneySystemID),
		WithMoneySystemIdentifier(moneySystemID),
		WithState(state.NewEmptyState()),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)
	execFn := handleAddFeeCreditTx(feeCreditModule)

	attr := testfc.NewAddFCAttr(t, signer)
	tx := testfc.NewAddFC(t, signer, attr)

	sm, err := execFn(tx, attr, 10)
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*unit.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 49, fcr.Balance)                      // transferFC.amount (50) - transferFC.fee (0) - addFC.fee (1)
	require.EqualValues(t, tx.Hash(crypto.SHA256), fcr.Backlink) // backlink is set to txhash
	require.EqualValues(t, 11, fcr.Timeout)                      // transferFC.latestAdditionTime + 1
	require.EqualValues(t, 0, fcr.Locked)                        // new unit is created in unlocked status

}

func TestAddFC_UpdateExistingFeeCreditRecord(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
	s := state.NewEmptyState()
	feeCreditModule, err := NewFeeCreditModule(
		WithSystemIdentifier(moneySystemID),
		WithMoneySystemIdentifier(moneySystemID),
		WithState(s),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)
	execFn := handleAddFeeCreditTx(feeCreditModule)

	attr := testfc.NewAddFCAttr(t, signer)
	tx := testfc.NewAddFC(t, signer, attr)
	existingFCR := &unit.FeeCreditRecord{Balance: 10, Backlink: nil, Locked: 1}
	require.NoError(t, s.Apply(state.AddUnit(tx.UnitID(), nil, existingFCR)))

	sm, err := execFn(tx, attr, 10)
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*unit.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 59, fcr.Balance)                      // existing (10) + transferFC.amount (50) - transferFC.fee (0) - addFC.fee (1)
	require.EqualValues(t, tx.Hash(crypto.SHA256), fcr.Backlink) // backlink is set to txhash
	require.EqualValues(t, 11, fcr.Timeout)                      // transferFC.latestAdditionTime + 1
	require.EqualValues(t, 0, fcr.Locked)                        // unit is unlocked
}
