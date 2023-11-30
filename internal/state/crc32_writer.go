package state

import (
	"hash"
	"hash/crc32"
	"io"
)

type CRC32Writer struct {
	hasher hash.Hash32
	writer io.Writer
}

func NewCRC32Writer(writer io.Writer) *CRC32Writer {
	return &CRC32Writer{
		hasher: crc32.NewIEEE(),
		writer: writer,
	}
}

func (c *CRC32Writer) Write(p []byte) (n int, err error) {
	n, err = c.writer.Write(p)
	c.hasher.Write(p)
	return
}

func (c *CRC32Writer) Sum() uint32 {
	return c.hasher.Sum32()
}
