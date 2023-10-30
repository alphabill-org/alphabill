package util

import (
	"math/rand"
)

func ShuffleSliceCopy[T any](src []T) []T {
	dst := make([]T, len(src))
	copy(dst, src)
	rand.Shuffle(len(dst), func(i, j int) { dst[i], dst[j] = dst[j], dst[i] })
	return dst
}
