package state

import (
	"hash"
	"hash/crc32"
	"io"
)

type CRC32Reader struct {
	hasher         hash.Hash32
	reader         io.Reader
	buf            []byte
	checksumLength int
}

func NewCRC32Reader(reader io.Reader, checksumLength int) *CRC32Reader {
	return &CRC32Reader{
		hasher:         crc32.NewIEEE(),
		reader:         reader,
		buf:            make([]byte, 0, checksumLength),
		checksumLength: checksumLength,
	}
}

// A hacky way to calculate the checksum of a data stream that has
// checksum appended to the end. Makes sure that the last
// checksumLength bytes are not included in the checksum calculation.
func (c *CRC32Reader) Read(p []byte) (n int, err error) {
	n, err = c.reader.Read(p)

	if n >= c.checksumLength {
		// Got more than checksumLength bytes, no checksum in
		// previously read data
		checksumStart := n - c.checksumLength
		_, _ = c.hasher.Write(c.buf)             // #nosec G104
		_, _ = c.hasher.Write(p[:checksumStart]) // #nosec G104

		// Store potential checksum
		c.buf = c.buf[:c.checksumLength]
		copy(c.buf, p[checksumStart:n])
	} else {
		// Got less than checksumLength bytes, some of the
		// checksum potentially in the previously read data
		c.buf = append(c.buf, p[:n]...)

		if len(c.buf) > c.checksumLength {
			// Add bytes that are certainly not checksum bytes
			checksumStart := len(c.buf) - c.checksumLength
			_, _ = c.hasher.Write(c.buf[:checksumStart]) // #nosec G104
			c.buf = c.buf[checksumStart:]
		}
	}

	return n, err
}

func (c *CRC32Reader) Sum() uint32 {
	return c.hasher.Sum32()
}
