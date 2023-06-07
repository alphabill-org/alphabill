package wallet

import (
	"fmt"
	"math"
)

// AddUint64 adds a list of uint64s together, returning an error if the sum overflows uint64.
func AddUint64(ns ...uint64) (uint64, error) {
	if len(ns) == 0 {
		return 0, nil
	}
	sum := ns[0]
	for i := 1; i < len(ns); i++ {
		n := ns[i]
		if n > math.MaxUint64-sum {
			return 0, fmt.Errorf("uint64 sum overflow: %v", ns)
		}
		sum += n
	}

	return sum, nil
}
