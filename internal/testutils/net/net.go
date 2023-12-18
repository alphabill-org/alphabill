package net

import (
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// SharedPortManager Instance of PortManager to be shared between tests to keep track of used ports
var SharedPortManager = &PortManager{
	usedPorts: make(map[int]bool),
}

type PortManager struct {
	usedPorts map[int]bool
	mutex     sync.Mutex
}

func (pm *PortManager) GetFreePort() (int, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	for {
		addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			return 0, err
		}

		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return 0, err
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()

		if !pm.usedPorts[port] {
			pm.usedPorts[port] = true
			return port, nil
		}
	}
}

func (pm *PortManager) GetRandomFreePort(t *testing.T) int {
	port, err := pm.GetFreePort()
	require.NoError(t, err)
	return port
}
