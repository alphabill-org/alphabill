package types

import (
	"bytes"
	"errors"
	"io"

	"github.com/fxamacker/cbor/v2"
)

type (
	RawCBOR []byte

	cborHandler struct {
		encMode cbor.EncMode
	}
)

var (
	Cbor = cborHandler{}

	cborNil = []byte{0xf6}
)

/*
Set Core Deterministic Encoding as standard. See <https://www.rfc-editor.org/rfc/rfc8949.html#name-deterministically-encoded-c>.
*/
func (c *cborHandler) cborEncoder() (cbor.EncMode, error) {
	if c.encMode != nil {
		return c.encMode, nil
	}
	encMode, err := cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	c.encMode = encMode
	return encMode, nil
}

func (c cborHandler) Marshal(v any) ([]byte, error) {
	enc, err := c.cborEncoder()
	if err != nil {
		return nil, err
	}
	return enc.Marshal(v)
}

func (c cborHandler) Unmarshal(data []byte, v any) error {
	return cbor.Unmarshal(data, v)
}

func (c cborHandler) GetEncoder(w io.Writer) (*cbor.Encoder, error) {
	enc, err := c.cborEncoder()
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

// MarshalCBOR returns r or CBOR nil if r is empty.
func (r RawCBOR) MarshalCBOR() ([]byte, error) {
	if len(r) == 0 {
		return cborNil, nil
	}
	return r, nil
}

// UnmarshalCBOR copies data into r unless it's CBOR "nil marker" - in that
// case r is set to empty slice.
func (r *RawCBOR) UnmarshalCBOR(data []byte) error {
	if r == nil {
		return errors.New("UnmarshalCBOR on nil pointer")
	}
	if bytes.Equal(data, cborNil) {
		*r = (*r)[0:0]
	} else {
		*r = append((*r)[0:0], data...)
	}
	return nil
}
