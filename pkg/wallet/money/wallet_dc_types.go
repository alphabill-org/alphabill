package money

import (
	"sync"

	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/holiman/uint256"
)

// DcBillGroup helper struct for grouped dc bills and their aggregate data
type DcBillGroup struct {
	DcBills   []*Bill
	ValueSum  uint64
	DcNonce   []byte
	DcTimeout uint64
}

// dcWaitGroup helper struct to support blocking collect dust feature.
type dcWaitGroup struct {
	// mu lock for modifying swaps field
	mu sync.Mutex

	// wg incremented once per expected swap, and decremented once per received swap bill
	wg sync.WaitGroup

	// swaps list of expected transactions to be received during dc process
	// key - dc nonce;
	// value - timeout block number
	swaps map[uint256.Int]uint64
}

// expectedSwap helper struct to support blocking collect dust feature
type expectedSwap struct {
	dcNonce []byte
	timeout uint64
}

func newDcWaitGroup() *dcWaitGroup {
	return &dcWaitGroup{swaps: map[uint256.Int]uint64{}}
}

// AddExpectedSwaps increments wg and records expected swap data for each expected swap.
func (wg *dcWaitGroup) AddExpectedSwaps(swaps []expectedSwap) {
	wg.mu.Lock()
	defer wg.mu.Unlock()
	for _, swap := range swaps {
		wg.addExpectedSwap(swap)
	}
}

// UpdateTimeout updates timeout from dc timeout to swap timeout after receiving dc bills and sending swap tx
func (wg *dcWaitGroup) UpdateTimeout(dcNonce []byte, timeout uint64) {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	key := *uint256.NewInt(0).SetBytes(dcNonce)
	_, exists := wg.swaps[key]
	if exists {
		wg.swaps[key] = timeout
	}
}

// ResetWaitGroup resets the expected swaps and decrements waitgroups
func (wg *dcWaitGroup) ResetWaitGroup() {
	wg.mu.Lock()
	defer wg.mu.Unlock()
	for k := range wg.swaps {
		wg.wg.Done()
		delete(wg.swaps, k)
	}
}

// addExpectedSwap increments wg and records expected swap data
func (wg *dcWaitGroup) addExpectedSwap(swap expectedSwap) {
	wg.wg.Add(1)
	wg.swaps[*util.BytesToUint256(swap.dcNonce)] = swap.timeout
}
