package network

import (
	"bufio"
	"encoding/binary"
	"io"

	"github.com/fxamacker/cbor/v2"
)

type CBORReader struct {
	r      *bufio.Reader
	buf    []byte
	closer io.Closer
}

func NewCBORReader(r io.Reader) *CBORReader {
	var closer io.Closer
	if c, ok := r.(io.Closer); ok {
		closer = c
	}
	return &CBORReader{bufio.NewReader(r), nil, closer}
}

func (pr *CBORReader) Read(msg any) error {
	// read data length
	length64, err := binary.ReadUvarint(pr.r)
	if err != nil {
		return err
	}
	length := int(length64)
	if length < 0 {
		return io.ErrShortBuffer
	}
	if len(pr.buf) < length {
		pr.buf = make([]byte, length)
	}
	buf := pr.buf[:length]
	if _, err := io.ReadFull(pr.r, buf); err != nil {
		return err
	}
	return cbor.Unmarshal(buf, msg)
}

func (pr *CBORReader) Close() error {
	if pr.closer != nil {
		return pr.closer.Close()
	}
	return nil
}
