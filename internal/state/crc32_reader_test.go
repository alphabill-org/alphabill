package state

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChecksumCalculation(t *testing.T) {
	type args struct {
		checksumLength int
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "checksumLength 0",
			args: args{
				checksumLength: 0,
			},
		},
		{
			name: "checksumLength 1",
			args: args{
				checksumLength: 1,
			},
		},
		{
			name: "checksumLength 4",
			args: args{
				checksumLength: 4,
			},
		},
		{
			name: "checksumLength 5",
			args: args{
				checksumLength: 5,
			},
		},
		{
			name: "checksumLength 10",
			args: args{
				checksumLength: 10,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			w := NewCRC32Writer(buf)
			r := NewCRC32Reader(buf, tt.args.checksumLength)

			data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
			// Data that goes through checksum calculation
			w.Write(data)

			// Data that should not go through checksum calculation
			buf.Write(data[:tt.args.checksumLength])

			readBytes := make([]byte, 3)
			for {
				_, err := r.Read(readBytes)
				if err == io.EOF {
					break
				}
			}

			require.Equal(t, w.Sum(), r.Sum())
		})
	}
}
