package fc

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/stretchr/testify/require"
)

func TestAddFC_AddNewFeeCredit(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
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

	sm, err := execFn(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 49, fcr.Balance)                      // transferFC.amount (50) - transferFC.fee (0) - addFC.fee (1)
	require.EqualValues(t, tx.Hash(crypto.SHA256), fcr.Backlink) // backlink is set to txhash
	require.EqualValues(t, 11, fcr.Timeout)                      // transferFC.latestAdditionTime + 1
	require.EqualValues(t, 0, fcr.Locked)                        // new unit is created in unlocked status

}

func TestAddFC_UpdateExistingFeeCreditRecord(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
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
	existingFCR := &fc.FeeCreditRecord{Balance: 10, Backlink: nil, Locked: 1}
	require.NoError(t, s.Apply(state.AddUnit(tx.UnitID(), nil, existingFCR)))

	sm, err := execFn(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 59, fcr.Balance)                      // existing (10) + transferFC.amount (50) - transferFC.fee (0) - addFC.fee (1)
	require.EqualValues(t, tx.Hash(crypto.SHA256), fcr.Backlink) // backlink is set to txhash
	require.EqualValues(t, 11, fcr.Timeout)                      // transferFC.latestAdditionTime + 1
	require.EqualValues(t, 0, fcr.Locked)                        // unit is unlocked
}
