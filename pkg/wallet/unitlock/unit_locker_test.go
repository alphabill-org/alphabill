package unitlock

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnitLocker(t *testing.T) {
	unitLocker := createUnitLocker(t)

	// lock bill id=1
	lockedBill1 := &LockedUnit{UnitID: []byte{1}, LockReason: ReasonAddFees}
	err := unitLocker.LockUnit(lockedBill1)
	require.NoError(t, err)

	// get bill id=1
	actualLockedBill1, err := unitLocker.GetUnit([]byte{1})
	require.NoError(t, err)
	require.Equal(t, lockedBill1, actualLockedBill1)

	// lock bill id=2
	lockedBill2 := &LockedUnit{UnitID: []byte{2}, LockReason: ReasonAddFees}
	err = unitLocker.LockUnit(lockedBill2)
	require.NoError(t, err)

	// get locked bills
	actualLockedBills, err := unitLocker.GetUnits()
	require.NoError(t, err)
	require.Equal(t, lockedBill1, actualLockedBills[0])
	require.Equal(t, lockedBill2, actualLockedBills[1])

	// unlock bill id=1
	err = unitLocker.UnlockUnit(lockedBill1.UnitID)
	require.NoError(t, err)

	// get bill id=1 returns nil
	actualLockedBill1, err = unitLocker.db.GetUnit(lockedBill1.UnitID)
	require.NoError(t, err)
	require.Nil(t, actualLockedBill1)

	// get locked bills returns only bill 2
	actualLockedBills, err = unitLocker.GetUnits()
	require.NoError(t, err)
	require.Len(t, actualLockedBills, 1)
	require.Equal(t, lockedBill2, actualLockedBills[0])
}

func createUnitLocker(t *testing.T) *UnitLocker {
	store := createUnitStore(t)
	return &UnitLocker{db: store}
}
