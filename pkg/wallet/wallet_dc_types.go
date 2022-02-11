package wallet

import (
	"github.com/holiman/uint256"
	"sync"
)

// dcMetadata container for grouping dcMetadata by nonce, persisted to db
type dcMetadata struct {
	DcValueSum  uint64 `json:"dcValueSum"` // only set by wallet managed dc job
	DcTimeout   uint64 `json:"dcTimeout"`
	SwapTimeout uint64 `json:"swapTimeout"`
}

// dcBillGroup helper struct for grouped dc bills and their aggregate data
type dcBillGroup struct {
	dcBills   []*bill
	valueSum  uint64
	dcNonce   []byte
	dcTimeout uint64
}

// dcWaitGroup helper struct to support blocking collect dust feature.
type dcWaitGroup struct {
	// wg incremented once per expected swap, and decremented once per received swap bill
	wg sync.WaitGroup

	// swaps list of expected transactions to be received during dc process
	// key - dc nonce;
	// value - timeout block height
	swaps map[uint256.Int]uint64
}

// expectedSwap helper struct to support blocking collect dust feature
type expectedSwap struct {
	dcNonce []byte
	timeout uint64
}

func newDcWaitGroup() dcWaitGroup {
	return dcWaitGroup{swaps: map[uint256.Int]uint64{}}
}

func (m *dcMetadata) isSwapRequired(blockHeight uint64, dcSum uint64) bool {
	return m.dcSumReached(dcSum) || m.timeoutReached(blockHeight)
}

func (m *dcMetadata) dcSumReached(dcSum uint64) bool {
	return m.DcValueSum > 0 && dcSum >= m.DcValueSum
}

func (m *dcMetadata) timeoutReached(blockHeight uint64) bool {
	return blockHeight == m.DcTimeout || blockHeight == m.SwapTimeout
}

// addExpectedSwaps increments wg and records expected swap data for each expected swap.
func (wg *dcWaitGroup) addExpectedSwaps(swaps []expectedSwap) {
	for _, swap := range swaps {
		wg.addExpectedSwap(swap)
	}
}

// addExpectedSwap increments wg and records expected swap data
func (wg *dcWaitGroup) addExpectedSwap(swap expectedSwap) {
	wg.wg.Add(1)
	wg.swaps[*uint256.NewInt(0).SetBytes(swap.dcNonce)] = swap.timeout
}

// removeSwap decrements wg and removes expected swap data
func (wg *dcWaitGroup) removeSwap(dcNonce uint256.Int) {
	wg.wg.Done()
	delete(wg.swaps, dcNonce)
}

// updateTimeout updates timeout from dc timeout to swap timeout after receiving dc bills and sending swap tx
func (wg *dcWaitGroup) updateTimeout(dcNonce []byte, timeout uint64) {
	key := *uint256.NewInt(0).SetBytes(dcNonce)
	_, exists := wg.swaps[key]
	if exists {
		wg.swaps[key] = timeout
	}
}

func (wg *dcWaitGroup) resetWaitGroup() {
	for k := range wg.swaps {
		wg.wg.Done()
		delete(wg.swaps, k)
	}
}
