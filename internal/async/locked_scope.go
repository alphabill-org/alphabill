package async

import (
	"sync"
)

func LockedScope(l sync.Locker, action func()) {
	l.Lock()
	defer l.Unlock()
	action()
}
