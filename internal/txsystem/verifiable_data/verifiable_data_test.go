package verifiable_data

import (
	"crypto"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

var vdSystemIdentifier = []byte{0, 0, 0, 1}

func TestRegisterData_Ok(t *testing.T) {
	err := createVD(t).Execute(createTx())
	require.NoError(t, err)
}

func TestRegisterData_InvalidOwnerProof(t *testing.T) {
	vd := createVD(t)
	tx := createTx()
	tx.OwnerProof = script.PredicateAlwaysTrue()

	err := vd.Execute(tx)
	require.ErrorIs(t, err, ErrOwnerProofPresent)
}

func TestRegisterData_WithDuplicate(t *testing.T) {
	vd := createVD(t)
	tx := createTx()
	err := vd.Execute(tx)
	require.NoError(t, err)

	// send duplicate
	err = vd.Execute(tx)
	require.ErrorContains(t, err, "could not add item")
}

func createTx() *transaction.Transaction {
	hasher := crypto.SHA256.New()
	hasher.Write(test.RandomBytes(32))
	id := hasher.Sum(nil)
	tx := &transaction.Transaction{
		SystemId: vdSystemIdentifier,
		UnitId:   id,
		Timeout:  2,
	}
	return tx
}

func createVD(t *testing.T) *txSystem {
	vd, err := New()
	require.NoError(t, err)
	return vd
}
