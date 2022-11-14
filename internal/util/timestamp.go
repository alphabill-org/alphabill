package util

import "time"

func MakeTimestamp() uint64 {
	return uint64(time.Now().UnixMilli())
}
