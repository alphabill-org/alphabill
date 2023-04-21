package util

import "time"

// GenesisTime min timestamp Thursday, April 20, 2023 6:11:24 AM GMT+00:00
const GenesisTime uint64 = 1681971084

// MakeTimestamp returns timestamp in seconds from epoch
func MakeTimestamp() uint64 {
	// Epoch in seconds
	return uint64(time.Now().Unix())
}
