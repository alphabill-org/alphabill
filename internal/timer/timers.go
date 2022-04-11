package timer

import (
	"sync"
	"time"

	log "gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
)

var logger = log.CreateForPackage()

type (
	// Timers keeps track of multiple time.Timer instances.
	// When one of the Timers expires then it will be sent on C.
	Timers struct {
		wg     *sync.WaitGroup
		timers map[string]*NamedTimer
		C      chan *NamedTimer
	}

	// NamedTimer is a time.Timer with a Name.
	NamedTimer struct {
		Name     string
		Duration time.Duration
		timer    *time.Timer
		cancelCh chan interface{}
	}
)

func (t *NamedTimer) loop(respChan chan<- *NamedTimer, wg *sync.WaitGroup) {
	select {
	case <-t.timer.C:
		respChan <- t
		wg.Done()
	case <-t.cancelCh:
		if !t.timer.Stop() {
			// drain the channel
			<-t.timer.C
		}
		t.timer.Reset(t.Duration)
		wg.Done()
	}
}

func NewTimers() *Timers {
	return &Timers{
		wg:     &sync.WaitGroup{},
		timers: make(map[string]*NamedTimer),
		C:      make(chan *NamedTimer),
	}
}

func (t *Timers) Start(id string, d time.Duration) {
	nt := &NamedTimer{
		Name:     id,
		Duration: d,
		timer:    time.NewTimer(d),
		cancelCh: make(chan interface{}, 1),
	}
	t.timers[id] = nt
	t.wg.Add(1)
	go nt.loop(t.C, t.wg)
}

func (t *Timers) Restart(id string) {
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

	nt.timer.Reset(nt.Duration)
	t.wg.Add(1)
	go nt.loop(t.C, t.wg)
}

func (t *Timers) WaitClose() {
	for _, timer := range t.timers {
		timer.cancelCh <- true
	}
	// ensure we have closed all Timers
	t.wg.Wait()
	close(t.C)
}
