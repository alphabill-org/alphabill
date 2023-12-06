package util

import "testing"

func Test_isBitSet(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		want  []bool
	}{
		{
			name:  "0x00",
			bytes: []byte{0x00}, //00000000
			want:  []bool{false, false, false, false, false, false, false, false},
		},
		{
			name:  "0xFF",
			bytes: []byte{0xFF}, // 11111111
			want:  []bool{true, true, true, true, true, true, true, true},
		},
		{
			name:  "0x00, 0xFF", // 00000000 11111111
			bytes: []byte{0x00, 0xFF},
			want: []bool{false, false, false, false, false, false, false, false,
				true, true, true, true, true, true, true, true},
		},
		{
			name:  "0x11, 0x12", // 00010001 00010010
			bytes: []byte{0x11, 0x12},
			want: []bool{false, false, false, true, false, false, false, true,
				false, false, false, true, false, false, true, false},
		},
		{
			name:  "0x11, 0x12, 0x11, 0x12", // 00010001 00010010 00010001 11111111
			bytes: []byte{0x11, 0x12, 0x11, 0xFF},
			want: []bool{false, false, false, true, false, false, false, true,
				false, false, false, true, false, false, true, false,
				false, false, false, true, false, false, false, true,
				true, true, true, true, true, true, true, true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			length := len(tt.bytes) * 8
			for i := 0; i < length; i++ {
				if got := IsBitSet(tt.bytes, i); got != tt.want[i] {
					t.Errorf("isBitSet() = %v, want %v", got, tt.want[i])
				}
			}
		})
	}
}
