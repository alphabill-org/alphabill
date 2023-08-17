package util

import (
	"github.com/alphabill-org/alphabill/internal/types"
)

// SameShardID creates a new UnitID from an existing UnitID and a hash
// value, preserving the shardID and the typeID components of the
// existing UnitID. The shardID component is the first 4 bytes of
// UnitID and the typeID component is the last typeIDLength bytes of
// UnitID.
func SameShardID(unitID types.UnitID, hashValue []byte, typeIDLength int) types.UnitID {
	unitIDBytes := []byte(unitID)
	newUnitIDBytes := make([]byte, 4)
	copy(newUnitIDBytes, unitIDBytes[:4])

	typelessUnitIDLength := len(unitID)-typeIDLength

	if len(hashValue) >= typelessUnitIDLength {
		newUnitIDBytes = append(newUnitIDBytes, hashValue[4:typelessUnitIDLength]...)
	} else {
		if len(hashValue) >= 5 {
			newUnitIDBytes = append(newUnitIDBytes, hashValue[4:]...)
		}
		for i := len(newUnitIDBytes); i < typelessUnitIDLength; i++ {
			newUnitIDBytes = append(newUnitIDBytes, 0)
		}
	}

	return append(newUnitIDBytes, unitIDBytes[typelessUnitIDLength:]...)
}
