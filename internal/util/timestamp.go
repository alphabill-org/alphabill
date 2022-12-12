package util

import "time"

func MakeTimestamp() uint64 {
	// In milliseconds will set the last three digits to 0
	return uint64(time.Now().UnixMilli())
}
