package wallet

import (
	"fmt"
	"math"
)

// AddUint64 adds a list of uint64s together, returning an error and a boolean indicator if the sum overflows uint64.
func AddUint64(ns ...uint64) (sum uint64, overflow bool, err error) {
	if len(ns) == 0 {
		return 0, false, nil
	}
	sum = ns[0]
	for i := 1; i < len(ns); i++ {
		n := ns[i]
		if n > math.MaxUint64-sum {
			overflow = true
		}
		sum += n
	}

	if overflow {
		err = fmt.Errorf("uint64 sum overflow: %v", ns)
	}

	return
}
