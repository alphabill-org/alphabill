package network

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

var ErrNotProtoMessage = errors.New("input isn't protobuf message")

type ProtobufWriter struct {
	w io.Writer
}

func NewProtoBufWriter(w io.Writer) *ProtobufWriter {
	return &ProtobufWriter{w}
}

func (pw *ProtobufWriter) Write(msg interface{}) (err error) {
	protoMsg, ok := msg.(proto.Message)
	if !ok {
		return ErrNotProtoMessage
	}
	data, err := proto.Marshal(protoMsg)
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

func (pw *ProtobufWriter) Close() error {
	if closer, ok := pw.w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
