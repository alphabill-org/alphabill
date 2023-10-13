package unitlock

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnitLocker(t *testing.T) {
	unitLocker := createUnitLocker(t)
	accountID := []byte{200}
	systemID := []byte{0, 0, 0, 0}

	// lock bill id=1
	lockedBill1 := NewLockedUnit(accountID, []byte{1}, []byte{2}, systemID, LockReasonAddFees)
	err := unitLocker.LockUnit(lockedBill1)
	require.NoError(t, err)

	// get bill id=1
	actualLockedBill1, err := unitLocker.GetUnit(accountID, []byte{1})
	require.NoError(t, err)
	require.Equal(t, lockedBill1, actualLockedBill1)

	// lock bill id=2
	lockedBill2 := NewLockedUnit(accountID, []byte{2}, []byte{3}, systemID, LockReasonAddFees)
	err = unitLocker.LockUnit(lockedBill2)
	require.NoError(t, err)

	// get locked bills
	actualLockedBills, err := unitLocker.GetUnits(accountID)
	require.NoError(t, err)
	require.Equal(t, lockedBill1, actualLockedBills[0])
	require.Equal(t, lockedBill2, actualLockedBills[1])

	// unlock bill id=1
	err = unitLocker.UnlockUnit(accountID, lockedBill1.UnitID)
	require.NoError(t, err)

	// get bill id=1 returns nil
	actualLockedBill1, err = unitLocker.GetUnit(accountID, lockedBill1.UnitID)
	require.NoError(t, err)
	require.Nil(t, actualLockedBill1)

	// get locked bills returns only bill 2
	actualLockedBills, err = unitLocker.GetUnits(accountID)
	require.NoError(t, err)
	require.Len(t, actualLockedBills, 1)
	require.Equal(t, lockedBill2, actualLockedBills[0])
}

func createUnitLocker(t *testing.T) *UnitLocker {
	store := createUnitStore(t)
	return &UnitLocker{db: store}
}
