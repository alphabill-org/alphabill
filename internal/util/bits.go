package util

func IsBitSet(bytes []byte, bitPosition int) bool {
	byteIndex := bitPosition / 8
	bitIndexInByte := bitPosition % 8
	return bytes[byteIndex]&byte(1<<(7-bitIndexInByte)) != byte(0)
}
