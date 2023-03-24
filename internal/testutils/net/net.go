package net

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func GetFreeRandomPort(t *testing.T) int {
	port, err := GetFreePort()
	require.NoError(t, err)
	return port
}

func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = l.Close()
	}()
	return l.Addr().(*net.TCPAddr).Port, nil
}
