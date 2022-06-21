package util

import (
	"math/rand"
	"time"
)

func ShuffleSliceCopy[T any](src []T) (dst []T) {
	dst = make([]T, len(src))
	copy(dst, src)
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(dst), func(i, j int) { dst[i], dst[j] = dst[j], dst[i] })
	return
}
