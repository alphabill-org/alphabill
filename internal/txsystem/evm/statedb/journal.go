package statedb

import (
	"github.com/ethereum/go-ethereum/common"
)

// journal contains the list of state modifications applied since the last state
// commit. These are tracked to be able to be reverted in the case of an execution
// exception or request for reversal.
type journal struct {
	entries []*common.Address      // Current changes tracked by the journal
	dirties map[common.Address]int // Dirty accounts and the number of changes
}

// newJournal creates a new initialized journal.
func newJournal() *journal {
	return &journal{
		entries: make([]*common.Address, 0),
		dirties: make(map[common.Address]int),
	}
}

// append inserts a new modification entry to the end of the change journal.
func (j *journal) append(entry *common.Address) {
	j.entries = append(j.entries, entry)
	if addr := entry; addr != nil {
		j.dirties[*addr]++
	}
}

// getModifiedUnits sort the dirty addresses for deterministic iteration
func (j *journal) getModifiedUnits() map[common.Address]int {
	return j.dirties
}

// revert removes reverted units from journal
func (j *journal) revert(snapshot int) {
	for i := len(j.entries) - 1; i >= snapshot; i-- {
		// Drop any dirty tracking induced by the change
		if addr := j.entries[i]; addr != nil {
			if j.dirties[*addr]--; j.dirties[*addr] == 0 {
				delete(j.dirties, *addr)
			}
		}
	}
	j.entries = j.entries[:snapshot]
}

// length returns the current number of entries in the journal.
func (j *journal) length() int {
	return len(j.entries)
}
