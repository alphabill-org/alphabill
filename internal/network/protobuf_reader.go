package network

import (
	"bufio"
	"encoding/binary"
	"io"

	"google.golang.org/protobuf/proto"
)

type ProtobufReader struct {
	r      *bufio.Reader
	buf    []byte
	closer io.Closer
}

func NewProtoBufReader(r io.Reader) *ProtobufReader {
	var closer io.Closer
	if c, ok := r.(io.Closer); ok {
		closer = c
	}
	return &ProtobufReader{bufio.NewReader(r), nil, closer}
}

func (pr *ProtobufReader) Read(msg proto.Message) error {
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
	return proto.Unmarshal(buf, msg)
}

func (pr *ProtobufReader) Close() error {
	if pr.closer != nil {
		return pr.closer.Close()
	}
	return nil
}
