package unitlock

import (
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/validator/internal/types"
)

const unitStoreFileName = "locked-units.db"

func TestUnitStore_GetSetDeleteUnits(t *testing.T) {
	s := createUnitStore(t)
	unitID := []byte{1}
	unit2ID := []byte{2}
	unit3ID := []byte{3}
	accountID := []byte{4}
	account2ID := []byte{5}
	txHash := []byte{6}
	systemID := []byte{0, 0, 0, 0}

	// verify missing account for unit returns nil and no error
	unit, err := s.GetUnit(accountID, unitID)
	require.NoError(t, err)
	require.Nil(t, unit)

	// store units
	unit = NewLockedUnit(accountID, unitID, txHash, systemID, LockReasonCollectDust, &Transaction{
		TxOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				Type:           money.PayloadTypeTransDC,
				ClientMetadata: &types.ClientMetadata{Timeout: 1000},
			},
		},
		TxHash: txHash,
	})
	err = s.PutUnit(unit)
	require.NoError(t, err)

	unit2 := NewLockedUnit(accountID, unit2ID, txHash, systemID, LockReasonCollectDust, &Transaction{
		TxOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				Type:           money.PayloadTypeTransDC,
				ClientMetadata: &types.ClientMetadata{Timeout: 1000},
			},
		},
		TxHash: txHash,
	})
	err = s.PutUnit(unit2)
	require.NoError(t, err)

	// verify stored unit equals actual unit
	storedUnit, err := s.GetUnit(accountID, unitID)
	require.NoError(t, err)
	require.Equal(t, unit, storedUnit)

	// verify get all units returns both units
	units, err := s.GetUnits(accountID)
	require.NoError(t, err)
	require.Contains(t, units, unit)
	require.Contains(t, units, unit2)

	// delete second unit
	err = s.DeleteUnit(accountID, unit2ID)
	require.NoError(t, err)

	// verify second unit is deleted
	unit, err = s.GetUnit(accountID, unit2ID)
	require.NoError(t, err)
	require.Nil(t, unit)

	// verify first unit is not deleted
	unit, err = s.GetUnit(accountID, unitID)
	require.NoError(t, err)
	require.NotNil(t, unit)
	require.Equal(t, unitID, unit.UnitID)

	// verify second account has no units
	units, err = s.GetUnits(account2ID)
	require.NoError(t, err)
	require.Len(t, units, 0)

	unit3 := NewLockedUnit(account2ID, unit3ID, txHash, systemID, LockReasonCollectDust, &Transaction{
		TxOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				Type:           money.PayloadTypeTransDC,
				ClientMetadata: &types.ClientMetadata{Timeout: 1000},
			},
		},
		TxHash: txHash,
	})
	err = s.PutUnit(unit3)
	require.NoError(t, err)

	// verify unit3 is stored
	actualUnit3, err := s.GetUnit(account2ID, unit3ID)
	require.NoError(t, err)
	require.Equal(t, unit3, actualUnit3)

	// verify get all units returns only unit3 for second account
	units, err = s.GetUnits(account2ID)
	require.NoError(t, err)
	require.Equal(t, []*LockedUnit{unit3}, units)
}

func createUnitStore(t *testing.T) *BoltStore {
	dbFile := filepath.Join(t.TempDir(), unitStoreFileName)
	store, err := NewBoltStore(dbFile)
	require.NoError(t, err)
	return store
}
