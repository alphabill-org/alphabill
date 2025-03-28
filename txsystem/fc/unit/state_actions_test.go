package unit

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/stretchr/testify/require"
)

var (
	id             = []byte{4}
	ownerPredicate = types.PredicateBytes{1, 2, 3}
)

func TestAddCredit_OK(t *testing.T) {
	fcr := &fc.FeeCreditRecord{
		Balance:        1,
		Counter:        10,
		MinLifetime:    2,
		OwnerPredicate: ownerPredicate,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, fcr))
	require.NoError(t, err)

	// verify unit is in state tree
	unit, err := tr.GetUnit(id, false)
	require.NoError(t, err)
	unitData, ok := unit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, fcr, unitData)
}

func TestDelCredit_OK(t *testing.T) {
	fcr := &fc.FeeCreditRecord{
		Balance:     1,
		Counter:     10,
		MinLifetime: 2,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, fcr))
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
	fcr := &fc.FeeCreditRecord{
		Balance:        1,
		Counter:        9,
		MinLifetime:    2,
		OwnerPredicate: ownerPredicate,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, fcr))
	require.NoError(t, err)

	// increment credit balance
	err = tr.Apply(IncrCredit(id, 99, 200))
	require.NoError(t, err)

	// verify balance is incremented
	unit, err := tr.GetUnit(id, false)
	require.NoError(t, err)
	unitData := unit.Data().(*fc.FeeCreditRecord)
	require.EqualValues(t, 100, unitData.Balance)
	require.EqualValues(t, 200, unitData.MinLifetime)
	require.EqualValues(t, 10, unitData.Counter)
	require.EqualValues(t, ownerPredicate, unitData.OwnerPredicate)
}

func TestDecrCredit_OK(t *testing.T) {
	fcr := &fc.FeeCreditRecord{
		Balance:     100,
		Counter:     10,
		MinLifetime: 2,
	}
	tr := state.NewEmptyState()

	// add credit unit to state tree
	err := tr.Apply(AddCredit(id, fcr))
	require.NoError(t, err)

	// decrement credit balance
	err = tr.Apply(DecrCredit(id, 10))
	require.NoError(t, err)

	// verify balance is decrement
	unit, err := tr.GetUnit(id, false)
	require.NoError(t, err)
	unitFCR := unit.Data().(*fc.FeeCreditRecord)
	require.EqualValues(t, 90, unitFCR.Balance)

	// and minLifetime and counter values are not changed
	require.Equal(t, fcr.MinLifetime, unitFCR.MinLifetime)
	require.Equal(t, fcr.Counter, unitFCR.Counter)
}
