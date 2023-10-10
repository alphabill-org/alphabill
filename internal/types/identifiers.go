package types

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/util"
)

type SystemID []byte
type SystemID32 uint32
type UnitID []byte

// NewUnitID creates a new UnitID consisting of a shardPart, unitPart and typePart.
func NewUnitID(unitIDLength int, shardPart []byte, unitPart []byte, typePart []byte) UnitID {
	unitID := make([]byte, unitIDLength)

	// The number of bytes to reserve for typePart in the new UnitID.
	typePartLength := len(typePart)
	// The number of bytes to reserve for unitPart in the new UnitID.
	unitPartLength := unitIDLength - typePartLength
	// The number of bytes to overwrite in the unitPart of the new UnitID with the shardPart.
	shardPartLength := 0

	// Copy unitPart, leaving zero bytes in the beginning in case
	// unitPart is shorter than unitPartLength.
	unitPartStart := util.Max(0, unitPartLength-len(unitPart))
	copy(unitID[unitPartStart:], unitPart)

	// Copy typePart
	copy(unitID[unitPartLength:], typePart)

	// Copy shardPart, overwriting shardPartLength bytes at the beginning of unitPart.
	copy(unitID, shardPart[:shardPartLength])

	return unitID
}

func (uid UnitID) Compare(key UnitID) int {
	return bytes.Compare(uid, key)
}

func (uid UnitID) String() string {
	return fmt.Sprintf("%X", []byte(uid))
}

func (uid UnitID) Eq(id UnitID) bool {
	return bytes.Equal(uid, id)
}

func (uid UnitID) HasType(typePart []byte) bool {
	return bytes.HasSuffix(uid, typePart)
}

func (sid SystemID) ToSystemID32() SystemID32 {
	return SystemID32(util.BytesToUint32(sid))
}

func (sid SystemID32) ToSystemID() SystemID {
	return util.Uint32ToBytes(uint32(sid))
}
