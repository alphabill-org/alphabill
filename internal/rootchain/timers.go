package rootchain

import (
	"sync"
	"time"
)

type (
	// timers keeps track of multiple time.Timer instances.
	// When one of the timers expires then it will be sent on C.
	timers struct {
		wg     *sync.WaitGroup
		timers map[string]*namedTimer
		C      chan *namedTimer
	}

	// namedTimer is a time.Timer with a name.
	namedTimer struct {
		name     string
		duration time.Duration
		timer    *time.Timer
		cancelCh chan interface{}
	}
)

func (t *namedTimer) loop(respChan chan<- *namedTimer, wg *sync.WaitGroup) {
	select {
	case <-t.timer.C:
		respChan <- t
		wg.Done()
	case <-t.cancelCh:
		if !t.timer.Stop() {
			// drain the channel
			<-t.timer.C
		}
		t.timer.Reset(t.duration)
		wg.Done()
	}
}

func NewTimers() *timers {
	return &timers{
		wg:     &sync.WaitGroup{},
		timers: make(map[string]*namedTimer),
		C:      make(chan *namedTimer),
	}
}

func (t *timers) Start(id string, d time.Duration) {
	nt := &namedTimer{
		name:     id,
		duration: d,
		timer:    time.NewTimer(d),
		cancelCh: make(chan interface{}, 1),
	}
	t.timers[id] = nt
	t.wg.Add(1)
	go nt.loop(t.C, t.wg)
}

func (t *timers) Restart(id string) {
	nt, found := t.timers[id]
	if !found {
		logger.Warning("Timer %v not found", id)
		return
	}
	nt.cancelCh <- true
	if !nt.timer.Stop() {
		select {
		// drain the cancel channel if the timer is already executed
		case <-nt.cancelCh:
		}
	}

	nt.timer.Reset(nt.duration)
	t.wg.Add(1)
	go nt.loop(t.C, t.wg)
}

func (t *timers) WaitClose() {
	for _, timer := range t.timers {
		timer.cancelCh <- true
	}
	// ensure we have closed all timers
	t.wg.Wait()
	close(t.C)
}
