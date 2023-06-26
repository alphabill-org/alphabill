package util

import (
	"github.com/alphabill-org/alphabill/internal/types"
)

// SameShardID creates ID that resides in the same shard.
// By taking first 4 bytes from id and last 28 bytes from the hashValue.
func SameShardID(id types.UnitID, hashValue []byte) types.UnitID {
	return SameShardIDBytes(id, hashValue)
}

func SameShardIDBytes(id types.UnitID, hashValue []byte) []byte {
	idBytes := []byte(id)
	newIdBytes := make([]byte, 4)
	copy(newIdBytes, idBytes[:4])

	if len(hashValue) >= 32 {
		newIdBytes = append(newIdBytes, hashValue[4:32]...)
	} else {
		if len(hashValue) >= 5 {
			newIdBytes = append(newIdBytes, hashValue[4:]...)
		}
		for i := len(newIdBytes); i < 32; i++ {
			newIdBytes = append(newIdBytes, 0)
		}
	}
	return newIdBytes
}
