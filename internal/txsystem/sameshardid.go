package txsystem

import "github.com/holiman/uint256"

// sameShardId creates ID that resides in the same shard.
// By taking first 4 bytes from id and last 28 bytes from the hashValue.
func sameShardId(id *uint256.Int, hashValue []byte) *uint256.Int {
	idBytes := id.Bytes32()
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
	return uint256.NewInt(0).SetBytes(newIdBytes)
}
