package types

import (
	"io"

	"github.com/fxamacker/cbor/v2"
)

type cborHandler struct{}

var Cbor = cborHandler{}

/*
Set Core Deterministic Encoding as standard. See <https://www.rfc-editor.org/rfc/rfc8949.html#name-deterministically-encoded-c>.
*/
func cborEncoder() (cbor.EncMode, error) {
	encMode, err := cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	return encMode, nil
}

func (c cborHandler) Marshal(v any) ([]byte, error) {
	enc, err := cborEncoder()
	if err != nil {
		return nil, err
	}
	return enc.Marshal(v)
}

func (c cborHandler) Unmarshal(data []byte, v any) error {
	return cbor.Unmarshal(data, v)
}

func (c cborHandler) GetEncoder(w io.Writer) (*cbor.Encoder, error) {
	enc, err := cborEncoder()
	if err != nil {
		return nil, err
	}
	return enc.NewEncoder(w), nil
}

func (c cborHandler) Encode(w io.Writer, v any) error {
	enc, err := c.GetEncoder(w)
	if err != nil {
		return err
	}
	return enc.Encode(v)
}

func (c cborHandler) GetDecoder(r io.Reader) *cbor.Decoder {
	return cbor.NewDecoder(r)
}

func (c cborHandler) Decode(r io.Reader, v any) error {
	return c.GetDecoder(r).Decode(v)
}
