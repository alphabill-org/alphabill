package unit

import (
	"testing"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/testutils"
	"github.com/stretchr/testify/require"
)

var (
	id    = []byte{4}
	owner = predicates.PredicateBytes{1, 2, 3}
)

func TestAddCredit_OK(t *testing.T) {
	hash := test.RandomBytes(32)
	fcr := &FeeCreditRecord{
		Balance:  1,
		Backlink: hash,
		Timeout:  2,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, owner, fcr))
	require.NoError(t, err)

	// verify unit is in state tree
	unit, err := tr.GetUnit(id, false)
	require.NoError(t, err)
	require.Equal(t, owner, unit.Bearer())
	require.Equal(t, hash, unit.Data().(*FeeCreditRecord).Backlink)
	require.Equal(t, fcr, unit.Data())
}

func TestDelCredit_OK(t *testing.T) {
	fcr := &FeeCreditRecord{
		Balance:  1,
		Backlink: test.RandomBytes(32),
		Timeout:  2,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, owner, fcr))
	require.NoError(t, err)

	// del credit unit from state tree
	err = tr.Apply(DelCredit(id))
	require.NoError(t, err)
	_, hash, err := tr.CalculateRoot()
	require.NoError(t, err)
	// verify state tree is empty
	require.Nil(t, hash)
}

func TestIncrCredit_OK(t *testing.T) {
	h := test.RandomBytes(32)
	fcr := &FeeCreditRecord{
		Balance:  1,
		Backlink: test.RandomBytes(32),
		Timeout:  2,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, owner, fcr))
	require.NoError(t, err)

	// increment credit balance
	err = tr.Apply(IncrCredit(id, 99, 200, h))
	require.NoError(t, err)

	// verify balance is incremented
	unit, err := tr.GetUnit(id, false)
	require.NoError(t, err)
	unitFCR := unit.Data().(*FeeCreditRecord)
	require.EqualValues(t, 100, unitFCR.Balance)
	require.EqualValues(t, 200, unitFCR.Timeout)
	require.Equal(t, h, unitFCR.Backlink)
	require.Equal(t, owner, unit.Bearer())
}

func TestDecrCredit_OK(t *testing.T) {
	fcr := &FeeCreditRecord{
		Balance:  1,
		Backlink: test.RandomBytes(32),
		Timeout:  2,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, owner, fcr))
	require.NoError(t, err)

	// decrement credit balance
	err = tr.Apply(DecrCredit(id, 101))
	require.NoError(t, err)

	// verify balance is decrement
	unit, err := tr.GetUnit(id, false)
	require.NoError(t, err)
	unitFCR := unit.Data().(*FeeCreditRecord)
	require.EqualValues(t, -100, unitFCR.Balance) // fcr and go negative

	// and timeout and hash are not changed
	require.Equal(t, fcr.Timeout, unitFCR.Timeout)
	require.Equal(t, fcr.Backlink, unitFCR.Backlink)
}
