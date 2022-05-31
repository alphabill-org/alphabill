package wallet

import (
	"context"
	"sync"
)

// syncFlagWrapper wrapper struct with mutex guarding synchronizing flag
type syncFlagWrapper struct {
	mu            sync.Mutex
	synchronizing bool // synchronizing true if wallet is currently synhronizing ledger, false otherwise
	ctx           context.Context
	ctxCancelFunc func()
}

func newSyncFlagWrapper() *syncFlagWrapper {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &syncFlagWrapper{ctx: ctx, ctxCancelFunc: cancelFunc}
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
