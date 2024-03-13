package wvm

import (
	"testing"
)

func Test_addr_encoding(t *testing.T) {
	testCases := []struct{ addr, size uint32 }{
		{addr: 0, size: 0},
		{addr: 0, size: 1},
		{addr: 1, size: 0},
		{addr: 1, size: 1},
		{addr: 0xFF00, size: 100},
		{addr: 0xFF0000CC, size: 500},
		{addr: 0, size: 0xFFFFFFFF},
		{addr: 0xFFFFFFFF, size: 0},
		{addr: 0xFFFFFFFF, size: 0xFFFFFFFF},
	}

	for x, tc := range testCases {
		ptr := newPointerSize(tc.addr, tc.size)
		addr, size := splitPointerSize(ptr)
		if addr != tc.addr {
			t.Errorf("[%d] address doesn't match after conversion: %d vs %d", x, tc.addr, addr)
		}
		if size != tc.size {
			t.Errorf("[%d] size doesn't match after conversion: %d vs %d", x, tc.size, size)
		}
	}
}
