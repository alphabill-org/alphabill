package network

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/alphabill-org/alphabill/types"
)

func serializeMsg(msg any) ([]byte, error) {
	data, err := types.Cbor.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshaling %T as CBOR: %w", msg, err)
	}
	length := uint64(len(data))
	lengthBytes := make([]byte, 8, 8+length)
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

	if err := types.Cbor.Decode(io.LimitReader(src, int64(length64)), msg); err != nil {
		return fmt.Errorf("decoding message data: %w", err)
	}

	return nil
}
