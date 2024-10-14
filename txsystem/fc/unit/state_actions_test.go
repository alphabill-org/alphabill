package unit

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/stretchr/testify/require"
)

var (
	id    = []byte{4}
	owner = types.PredicateBytes{1, 2, 3}
)

func TestAddCredit_OK(t *testing.T) {
	counter := uint64(10)
	fcr := &fc.FeeCreditRecord{
		Balance: 1,
		Counter: counter,
		Timeout: 2,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, owner, fcr))
	require.NoError(t, err)

	// verify unit is in state tree
	unit, err := tr.GetUnit(id, false)
	require.NoError(t, err)
	require.Equal(t, owner, unit.Owner())
	require.Equal(t, counter, unit.Data().(*fc.FeeCreditRecord).Counter)
	require.Equal(t, fcr, unit.Data())
}

func TestDelCredit_OK(t *testing.T) {
	fcr := &fc.FeeCreditRecord{
		Balance: 1,
		Counter: 10,
		Timeout: 2,
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
	counter := uint64(10)
	fcr := &fc.FeeCreditRecord{
		Balance: 1,
		Counter: counter,
		Timeout: 2,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, owner, fcr))
	require.NoError(t, err)

	// increment credit balance
	err = tr.Apply(IncrCredit(id, 99, 200))
	require.NoError(t, err)

	// verify balance is incremented
	unit, err := tr.GetUnit(id, false)
	require.NoError(t, err)
	unitFCR := unit.Data().(*fc.FeeCreditRecord)
	require.EqualValues(t, 100, unitFCR.Balance)
	require.EqualValues(t, 200, unitFCR.Timeout)
	require.Equal(t, counter+1, unitFCR.Counter)
	require.Equal(t, owner, unit.Owner())
}

func TestDecrCredit_OK(t *testing.T) {
	fcr := &fc.FeeCreditRecord{
		Balance: 100,
		Counter: 10,
		Timeout: 2,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, owner, fcr))
	require.NoError(t, err)

	// decrement credit balance
	err = tr.Apply(DecrCredit(id, 10))
	require.NoError(t, err)

	// verify balance is decrement
	unit, err := tr.GetUnit(id, false)
	require.NoError(t, err)
	unitFCR := unit.Data().(*fc.FeeCreditRecord)
	require.EqualValues(t, 90, unitFCR.Balance)

	// and timeout, counter and locked values are not changed
	require.Equal(t, fcr.Timeout, unitFCR.Timeout)
	require.Equal(t, fcr.Counter, unitFCR.Counter)
	require.Equal(t, fcr.Locked, unitFCR.Locked)
}
