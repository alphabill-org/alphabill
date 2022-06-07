package wallet

import (
	"sync"
)

// syncFlagWrapper wrapper struct with mutex guarding synchronizing flag
type syncFlagWrapper struct {
	mu            sync.Mutex
	synchronizing bool // synchronizing true if wallet is currently synhronizing ledger, false otherwise
	cancelSyncCh  chan bool
}

func newSyncFlagWrapper() *syncFlagWrapper {
	return &syncFlagWrapper{cancelSyncCh: make(chan bool, 1)}
}

func (w *syncFlagWrapper) setSynchronizing(synchronizing bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.synchronizing = synchronizing
}

func (w *syncFlagWrapper) isSynchronizing() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.synchronizing
}
