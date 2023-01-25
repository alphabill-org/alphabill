package money

import (
	"sync"

	"github.com/holiman/uint256"
)

// dcMetadata container for grouping dcMetadata by nonce, persisted to db
type dcMetadata struct {
	DcValueSum  uint64   `json:"dcValueSum"` // only set by wallet managed dc job
	BillIds     [][]byte `json:"billIds"`
	DcTimeout   uint64   `json:"dcTimeout"`
	SwapTimeout uint64   `json:"swapTimeout"`
}

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

func (m *dcMetadata) isSwapRequired(roundNumber uint64, dcSum uint64) bool {
	return m.dcSumReached(dcSum) || m.timeoutReached(roundNumber)
}

func (m *dcMetadata) dcSumReached(dcSum uint64) bool {
	return m.DcValueSum > 0 && dcSum >= m.DcValueSum
}

func (m *dcMetadata) timeoutReached(blockHeight uint64) bool {
	return blockHeight == m.DcTimeout || blockHeight == m.SwapTimeout
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
func (wg *dcWaitGroup) DecrementSwaps(tx TxContext, blockHeight uint64, accountIndex uint64) error {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	for dcNonce, timeout := range wg.swaps {
		exists, err := tx.ContainsBill(accountIndex, &dcNonce)
		if err != nil {
			return err
		}
		if exists || blockHeight >= timeout {
			wg.removeSwap(dcNonce)
		}
	}
	return nil
}

// removeSwap decrements wg and removes expected swap data
func (wg *dcWaitGroup) removeSwap(dcNonce uint256.Int) {
	delete(wg.swaps, dcNonce)
	wg.wg.Done()
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
	wg.swaps[*uint256.NewInt(0).SetBytes(swap.dcNonce)] = swap.timeout
}
