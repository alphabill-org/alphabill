package statedb

import (
	"github.com/ethereum/go-ethereum/common"
)

type journalEntry interface {
	// revert undoes the changes introduced by this journal entry.
	revert(*StateDB)
	// address returns the Ethereum address if the modified entry adjusted a unit in state DB.
	address() *common.Address
}

// journal contains the list of state modifications applied since the last state
// commit. These are tracked to be able to be reverted in the case of an execution
// exception or request for reversal.
type journal struct {
	entries []journalEntry         // Current changes tracked by the journal
	dirties map[common.Address]int // Dirty accounts and the number of changes
}

// newJournal creates a new initialized journal.
func newJournal() *journal {
	return &journal{
		dirties: make(map[common.Address]int),
	}
}

// append inserts a new modification entry to the end of the change journal.
func (j *journal) append(entry journalEntry) {
	j.entries = append(j.entries, entry)
	if addr := entry.address(); addr != nil {
		j.dirties[*addr]++
	}
}

// getModifiedUnits sort the dirty addresses for deterministic iteration
func (j *journal) getModifiedUnits() map[common.Address]int {
	return j.dirties
}

// revert removes reverted units from journal
func (j *journal) revert(stateDB *StateDB, snapshot int) {
	if snapshot > len(j.entries) {
		return
	}
	for i := len(j.entries) - 1; i >= snapshot; i-- {
		// Undo the changes made by the operation
		j.entries[i].revert(stateDB)
		// Drop any dirty tracking induced by the change
		if addr := j.entries[i].address(); addr != nil {
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

type (
	// Changes to the account trie.
	accountChange struct {
		account *common.Address
	}
	// refund change
	refundChange struct {
		prev uint64
	}
	// transient storage
	transientStorageChange struct {
		account       *common.Address
		key, prevalue common.Hash
	}
)

// address - returns address changed
func (ch accountChange) address() *common.Address {
	return ch.account
}

// revert - does nothing, this is handled by the state tree
func (ch accountChange) revert(s *StateDB) {
}

// address - not related to an account, return nil
func (ch refundChange) address() *common.Address {
	return nil
}

// revert - reverts to previous refund
func (ch refundChange) revert(s *StateDB) {
	s.refund = ch.prev
}

// address - not related to an account, return nil
func (ch transientStorageChange) address() *common.Address {
	return nil
}

func (ch transientStorageChange) revert(s *StateDB) {
	s.transientStorage.Set(*ch.account, ch.key, ch.prevalue)
}
