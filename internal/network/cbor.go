package network

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
)

func serializeMsg(msg any) ([]byte, error) {
	data, err := cbor.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshaling %T as CBOR: %w", msg, err)
	}
	length := uint64(len(data))
	lengthBytes := make([]byte, 8)
	bytesWritten := binary.PutUvarint(lengthBytes, length)
	return append(lengthBytes[:bytesWritten], data...), nil
}

func deserializeMsg(r io.Reader, msg any) error {
	src := bufio.NewReader(r)
	// read data length
	length64, err := binary.ReadUvarint(src)
	if err != nil {
		return fmt.Errorf("reading data length: %w", err)
	}
	if length64 == 0 {
		return fmt.Errorf("unexpected data length zero")
	}

	dec := cbor.NewDecoder(io.LimitReader(src, int64(length64)))
	if err := dec.Decode(msg); err != nil {
		return fmt.Errorf("decoding message data: %w", err)
	}

	return nil
}