package verifiable_data

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

var vdSystemIdentifier = []byte{0, 0, 0, 1}

func TestRegisterData_Ok(t *testing.T) {
	err := createVD(t).Execute(createTx(t))
	require.NoError(t, err)
}

func TestRegisterData_InvalidOwnerProof(t *testing.T) {
	vd := createVD(t)
	tx := &txsystem.Transaction{
		SystemId:       vdSystemIdentifier,
		UnitId:         make([]byte, 32),
		ClientMetadata: &txsystem.ClientMetadata{Timeout: 2},
	}
	genTx, err := txsystem.NewDefaultGenericTransaction(tx)
	require.NoError(t, err)
	tx.OwnerProof = script.PredicateAlwaysTrue()

	err = vd.Execute(genTx)
	require.ErrorIs(t, err, ErrOwnerProofPresent)
}

func TestRegisterData_Revert(t *testing.T) {
	vd := createVD(t)
	vdState, err := vd.State()
	require.NoError(t, err)
	err = vd.Execute(createTx(t))
	require.NoError(t, err)
	_, err = vd.State()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)
	require.NotEqual(t, vdState, vd.getState())
	vd.Revert()
	state, err := vd.State()
	require.NoError(t, err)
	require.Equal(t, vdState, state)
}

func TestRegisterData_WithDuplicate(t *testing.T) {
	vd := createVD(t)
	tx := createTx(t)
	err := vd.Execute(tx)
	require.NoError(t, err)

	// send duplicate
	err = vd.Execute(tx)
	require.ErrorContains(t, err, "could not add item")
}

func createTx(t *testing.T) txsystem.GenericTransaction {
	hasher := crypto.SHA256.New()
	hasher.Write(test.RandomBytes(32))
	id := hasher.Sum(nil)
	tx := &txsystem.Transaction{
		SystemId:       vdSystemIdentifier,
		UnitId:         id,
		ClientMetadata: &txsystem.ClientMetadata{Timeout: 2},
	}
	genTx, err := txsystem.NewDefaultGenericTransaction(tx)
	require.NoError(t, err)
	return genTx
}

func createVD(t *testing.T) *txSystem {
	vd, err := New(vdSystemIdentifier)
	require.NoError(t, err)
	return vd
}
