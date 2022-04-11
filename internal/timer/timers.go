package timer

import (
	"sync"
	"time"

	log "gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
)

var logger = log.CreateForPackage()

type (
	// Timers keeps track of multiple Task instances.
	// When one of the Task expires then it will be sent on C.
	Timers struct {
		wg     *sync.WaitGroup
		timers map[string]*Task
		C      chan *Task
	}

	// Task groups together a time.Timer, a Name of the timer, Duration and a cancel channel.
	Task struct {
		name     string
		duration time.Duration
		timer    *time.Timer
		cancelCh chan interface{}
	}
)

func (t *Task) Name() string {
	return t.name
}

func (t *Task) Duration() time.Duration {
	return t.duration
}

func (t *Task) run(respChan chan<- *Task, wg *sync.WaitGroup) {
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

func NewTimers() *Timers {
	return &Timers{
		wg:     &sync.WaitGroup{},
		timers: make(map[string]*Task),
		C:      make(chan *Task),
	}
}

func (t *Timers) Start(name string, d time.Duration) {
	nt := &Task{
		name:     name,
		duration: d,
		timer:    time.NewTimer(d),
		cancelCh: make(chan interface{}, 1),
	}
	t.timers[name] = nt
	t.wg.Add(1)
	go nt.run(t.C, t.wg)
}

func (t *Timers) Restart(name string) {
	nt, found := t.timers[name]
	if !found {
		logger.Warning("Timer %v not found", name)
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
	go nt.run(t.C, t.wg)
}

func (t *Timers) WaitClose() {
	for _, timer := range t.timers {
		timer.cancelCh <- true
	}
	// ensure we have closed all Timers
	t.wg.Wait()
	close(t.C)
}
