package network

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
)

type CBORWriter struct {
	w io.Writer
}

func NewCBORWriter(w io.Writer) *CBORWriter {
	return &CBORWriter{w}
}

func (pw *CBORWriter) Write(msg any) error {
	data, err := cbor.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal error, %w", err)
	}
	length := uint64(len(data))
	lengthBytes := make([]byte, 8)
	bytesWritten := binary.PutUvarint(lengthBytes, length)
	dataToWrite := append(lengthBytes[:bytesWritten], data...)
	_, err = pw.w.Write(dataToWrite)
	return err
}

func (pw *CBORWriter) Close() error {
	if closer, ok := pw.w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
