package money

import (
	"sync"
)

// dcBillGroup helper struct for grouped dc bills and their aggregate data
type dcBillGroup struct {
	dcBills   []*Bill
	valueSum  uint64
	dcNonce   []byte
	dcTimeout uint64
}

// dcWaitGroup helper struct to support blocking collect dust feature.
type dcWaitGroup struct {
	// mu lock for modifying swaps field
	mu sync.Mutex

	// wg incremented once per expected swap, and decremented once per received swap bill
	wg sync.WaitGroup

	// swaps list of expected transactions to be received during dc process
	// key - dc nonce;
	// value - timeout block height
	swaps map[string]expectedSwap
}

// expectedSwap helper struct to support blocking collect dust feature
type expectedSwap struct {
	dcNonce []byte
	timeout uint64
	dcSum   uint64
}

func newDcWaitGroup() *dcWaitGroup {
	return &dcWaitGroup{swaps: map[string]expectedSwap{}}
}

// AddExpectedSwaps increments wg and records expected swap data for each expected swap.
func (wg *dcWaitGroup) AddExpectedSwaps(swaps []expectedSwap) {
	wg.mu.Lock()
	defer wg.mu.Unlock()
	for _, swap := range swaps {
		wg.addExpectedSwap(swap)
	}
}

// DecrementSwaps decrement waitgroup after receiving expected swap bills, or timing out on dc/swap
func (wg *dcWaitGroup) DecrementSwaps(dcNonce string, blockHeight uint64) error {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	swap, exists := wg.swaps[dcNonce]
	if exists || blockHeight >= swap.timeout {
		wg.removeSwap(dcNonce)
	}
	return nil
}

// removeSwap decrements wg and removes expected swap data
func (wg *dcWaitGroup) removeSwap(dcNonce string) {
	delete(wg.swaps, dcNonce)
	wg.wg.Done()
}

// UpdateTimeout updates timeout from dc timeout to swap timeout after receiving dc bills and sending swap tx
func (wg *dcWaitGroup) UpdateTimeout(dcNonce []byte, timeout uint64) {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	key := string(dcNonce)
	swap, exists := wg.swaps[key]
	if exists {
		swap.timeout = timeout
		wg.swaps[key] = swap
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
	wg.swaps[string(swap.dcNonce)] = swap
}

func (wg *dcWaitGroup) getExpectedSwap(dcNonce string) expectedSwap {
	return wg.swaps[dcNonce]
}
