package state

import (
	"hash"
	"hash/crc32"
	"io"
)

const checksumLength = 5

type CRC32Reader struct {
	hasher hash.Hash32
	reader io.Reader
	buf    []byte
}

func NewCRC32Reader(reader io.Reader) *CRC32Reader {
	return &CRC32Reader{
		hasher: crc32.NewIEEE(),
		reader: reader,
		buf:    make([]byte, 0, checksumLength),
	}
}

// A hacky way to calculate the checksum of a data stream that has
// checksum appended to the end. Makes sure that the last
// checksumLength bytes are not included in the checksum calculation.
func (c *CRC32Reader) Read(p []byte) (n int, err error) {
	n, err = c.reader.Read(p)

	if n >= checksumLength {
		// Got more than checksumLength bytes, no checksum in
		// previously read data
		checksumStart := n - checksumLength
		c.hasher.Write(c.buf)
		c.hasher.Write(p[:checksumStart])

		// Store potential checksum
		c.buf = c.buf[:checksumLength]
		copy(c.buf, p[checksumStart:n])
	} else {
		// Got less than checksumLength bytes, some of the
		// checksum potentially in the previously read data
		c.buf = append(c.buf, p[:n]...)

		if len(c.buf) > checksumLength {
			// Add bytes that are certainly not checksum bytes
			checksumStart := len(c.buf) - checksumLength
			c.hasher.Write(c.buf[:checksumStart])
			c.buf = c.buf[checksumStart:]
		}
	}

	return n, err
}

func (c *CRC32Reader) Sum() uint32 {
	return c.hasher.Sum32()
}
