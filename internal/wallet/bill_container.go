package wallet

import (
	"github.com/holiman/uint256"
	"sync"
)

type (
	// billContainer wrapper struct around bills for thread safe access
	billContainer struct {
		mu    sync.RWMutex
		bills map[*uint256.Int]bill
	}

	bill struct {
		id     *uint256.Int
		value  uint64
		txHash []byte
	}
)

func NewBillContainer() *billContainer {
	return &billContainer{
		bills: map[*uint256.Int]bill{},
	}
}

func (c *billContainer) addBill(bill bill) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bills[bill.id] = bill
}

func (c *billContainer) getBalance() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sum := uint64(0)
	for _, b := range c.bills {
		sum += b.value // safe from overflow, there can only be 100B alphabills
	}
	return sum
}

func (c *billContainer) containsBill(id *uint256.Int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.bills[id]
	return exists
}

func (c *billContainer) removeBill(id *uint256.Int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.bills, id)
}

// removeBillIfExists checks if bill exists with read lock and only takes write lock if bill actually exists
func (c *billContainer) removeBillIfExists(id *uint256.Int) {
	if c.containsBill(id) {
		c.removeBill(id)
	}
}

func (c *billContainer) getBillWithMinValue(amount uint64) (bill, bool) {
	c.mu.RLock()
	c.mu.RUnlock()
	// TODO https://guardtime.atlassian.net/browse/AB-57
	// bill selection algorithm, swap background process
	for _, bill := range c.bills {
		if bill.value >= amount {
			return bill, true
		}
	}
	return bill{}, false
}
