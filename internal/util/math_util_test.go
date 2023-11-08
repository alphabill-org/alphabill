package util

import (
	"math"
	"testing"
)

func TestAddUint64(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args    []uint64
		want    uint64
		wantErr bool
	}{
		{nil, 0, false},
		{[]uint64{}, 0, false},
		{[]uint64{0}, 0, false},
		{[]uint64{1}, 1, false},
		{[]uint64{math.MaxUint64}, math.MaxUint64, false},
		{[]uint64{1, 2, 3}, 6, false},
		{[]uint64{1, math.MaxUint64}, 0, true},
		{[]uint64{math.MaxUint64, math.MaxUint64}, math.MaxUint64 - 1, true},
	}
	for _, c := range cases {
		got, overflow, err := AddUint64(c.args...)

		if err != nil && !c.wantErr {
			t.Errorf("AddUint64(%v) got unexpected error: %v", c.args, err)
		}
		if overflow && !c.wantErr {
			t.Errorf("AddUint64(%v) got unexpected overflow: %v", c.args, overflow)
		}
		if err == nil && c.wantErr {
			t.Errorf("AddUint64(%v) expected error but got none", c.args)
		}
		if got != c.want {
			t.Errorf("AddUint64(%v) got %d, want %d", c.args, got, c.want)
		}
	}

}
