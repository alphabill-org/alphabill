package txsystem

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var (
	id        = uint256.NewInt(4)
	owner     = rma.Predicate{1, 2, 3}
	stateHash = []byte("state hash")
)

func TestAddCredit_OK(t *testing.T) {
	fcr := &FeeCreditRecord{
		balance: 1,
		hash:    test.RandomBytes(32),
		timeout: 2,
	}
	tr, _ := rma.New(&rma.Config{HashAlgorithm: crypto.SHA256})

	// add credit unit to state tree
	err := tr.AtomicUpdate(AddCredit(id, owner, fcr, stateHash))
	require.NoError(t, err)

	// verify unit is in state tree
	unit, err := tr.GetUnit(id)
	require.NoError(t, err)
	require.Equal(t, owner, unit.Bearer)
	require.Equal(t, stateHash, unit.StateHash)
	require.Equal(t, fcr, unit.Data)
}

func TestDelCredit_OK(t *testing.T) {
	fcr := &FeeCreditRecord{
		balance: 1,
		hash:    test.RandomBytes(32),
		timeout: 2,
	}
	tr, _ := rma.New(&rma.Config{HashAlgorithm: crypto.SHA256})

	// add credit unit to state tree
	err := tr.AtomicUpdate(AddCredit(id, owner, fcr, stateHash))
	require.NoError(t, err)

	// del credit unit from state tree
	err = tr.AtomicUpdate(DelCredit(id))
	require.NoError(t, err)

	// verify state tree is empty
	require.Nil(t, tr.GetRootHash())
}

func TestIncrCredit_OK(t *testing.T) {
	h := test.RandomBytes(32)
	fcr := &FeeCreditRecord{
		balance: 1,
		hash:    test.RandomBytes(32),
		timeout: 2,
	}
	tr, _ := rma.New(&rma.Config{HashAlgorithm: crypto.SHA256})

	// add credit unit to state tree
	err := tr.AtomicUpdate(AddCredit(id, owner, fcr, stateHash))
	require.NoError(t, err)

	// increment credit balance
	err = tr.AtomicUpdate(IncrCredit(id, 99, 200, h))
	require.NoError(t, err)

	// verify balance is incremented
	unit, err := tr.GetUnit(id)
	require.NoError(t, err)
	unitFCR := unit.Data.(*FeeCreditRecord)
	require.EqualValues(t, 100, unitFCR.balance)
	require.EqualValues(t, 200, unitFCR.timeout)
	require.Equal(t, h, unitFCR.hash)
	require.Equal(t, h, unit.StateHash)
	require.Equal(t, owner, unit.Bearer)
}

func TestDecrCredit_OK(t *testing.T) {
	h := test.RandomBytes(32)
	fcr := &FeeCreditRecord{
		balance: 1,
		hash:    test.RandomBytes(32),
		timeout: 2,
	}
	tr, _ := rma.New(&rma.Config{HashAlgorithm: crypto.SHA256})

	// add credit unit to state tree
	err := tr.AtomicUpdate(AddCredit(id, owner, fcr, stateHash))
	require.NoError(t, err)

	// decrement credit balance
	err = tr.AtomicUpdate(DecrCredit(id, 101, h))
	require.NoError(t, err)

	// verify balance is decrement
	unit, err := tr.GetUnit(id)
	require.NoError(t, err)
	unitFCR := unit.Data.(*FeeCreditRecord)
	require.EqualValues(t, -100, unitFCR.balance) // fcr and go negative

	// and timeout and hash are not changed
	require.Equal(t, fcr.timeout, unitFCR.timeout)
	require.Equal(t, fcr.hash, unitFCR.hash)
}
