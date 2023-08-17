package unitlock

import (
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/stretchr/testify/require"
)

const unitStoreFileName = "locked-units.db"

func TestUnitStore_GetSetDeleteUnits(t *testing.T) {
	s := createUnitStore(t)
	unitID := []byte{1}
	unit2ID := []byte{2}

	// verify missing unit return nil and no error
	unit, err := s.GetUnit(unitID)
	require.NoError(t, err)
	require.Nil(t, unit)

	// store units
	unit = &LockedUnit{
		UnitID:     unitID,
		LockReason: ReasonCollectDust,
		Transactions: []*Transaction{{
			TxOrder:     nil,
			PayloadType: money.PayloadTypeTransDC,
			Timeout:     1000,
			TxHash:      []byte{5},
		}},
	}
	err = s.PutUnit(unit)
	require.NoError(t, err)

	unit2 := &LockedUnit{
		UnitID:     unit2ID,
		LockReason: ReasonCollectDust,
		Transactions: []*Transaction{{
			TxOrder:     nil,
			PayloadType: money.PayloadTypeTransDC,
			Timeout:     1000,
			TxHash:      []byte{6},
		}},
	}
	err = s.PutUnit(unit2)
	require.NoError(t, err)

	// verify stored unit equals actual unit
	storedUnit, err := s.GetUnit(unitID)
	require.NoError(t, err)
	require.Equal(t, unit, storedUnit)

	// verify get all units returns both units
	units, err := s.GetUnits()
	require.NoError(t, err)
	require.Contains(t, units, unit)
	require.Contains(t, units, unit2)

	// delete unit
	err = s.DeleteUnit(unitID)
	require.NoError(t, err)

	// verify unit is deleted
	unit, err = s.GetUnit(unitID)
	require.NoError(t, err)
	require.Nil(t, unit)
}

func createUnitStore(t *testing.T) *BoltStore {
	dbFile := filepath.Join(t.TempDir(), unitStoreFileName)
	store, err := NewBoltStore(dbFile)
	require.NoError(t, err)
	return store
}
