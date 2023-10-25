package network

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

type noCBOR struct {
	Name  string
	Value int
}

func (*noCBOR) MarshalCBOR() ([]byte, error) { return nil, errors.New("no CBOR for this type") }
func (*noCBOR) UnmarshalCBOR([]byte) error   { return errors.New("no CBOR for this type") }

func Test_serializeMsg(t *testing.T) {
	type testMsg struct {
		_     struct{} `cbor:",toarray"`
		Name  string
		Value int
	}

	t.Run("type doesn't support CBOR", func(t *testing.T) {
		b, err := serializeMsg(noCBOR{Name: "foo"})
		require.Nil(t, b)
		require.EqualError(t, err, `marshaling network.noCBOR as CBOR: no CBOR for this type`)
	})

	t.Run("success", func(t *testing.T) {
		msg := testMsg{Name: "foo", Value: 12}
		b, err := serializeMsg(msg)
		require.NoError(t, err)
		require.NotNil(t, b)

		var dest testMsg
		err = deserializeMsg(bytes.NewReader(b), &dest)
		require.NoError(t, err)
		require.Equal(t, msg, dest)
	})
}

func Test_deserializeMsg(t *testing.T) {
	type testMsg struct {
		_     struct{} `cbor:",toarray"`
		Name  string
		Value int
	}

	// create valid CBOR data of a message to use as input in tests
	msg := testMsg{Name: "foo", Value: 12}
	cborData, smerr := serializeMsg(msg)
	require.NoError(t, smerr)
	require.NotNil(t, cborData)

	t.Run("success", func(t *testing.T) {
		var dest testMsg
		require.NoError(t, deserializeMsg(bytes.NewReader(cborData), &dest))
		require.Equal(t, msg, dest)
	})

	t.Run("destination type doesn't support CBOR", func(t *testing.T) {
		var dest noCBOR
		err := deserializeMsg(bytes.NewReader(cborData), &dest)
		require.EqualError(t, err, `decoding message data: no CBOR for this type`)
	})

	t.Run("empty input", func(t *testing.T) {
		var dest testMsg
		err := deserializeMsg(bytes.NewReader(nil), &dest)
		require.EqualError(t, err, `reading data length: EOF`)
	})

	t.Run("data stream is shorter than expected", func(t *testing.T) {
		var dest testMsg
		err := deserializeMsg(bytes.NewReader(cborData[:len(cborData)-1]), &dest)
		require.EqualError(t, err, `decoding message data: unexpected EOF`)
	})

	t.Run("extra data in the data stream", func(t *testing.T) {
		// data stream has a "length" in it so the extra data should be ignored
		var dest testMsg
		err := deserializeMsg(bytes.NewReader(append(slices.Clone(cborData), 2, 3, 4)), &dest)
		require.NoError(t, err)
	})

	t.Run("length byte -1", func(t *testing.T) {
		data := slices.Clone(cborData)
		data[0]--
		var dest testMsg
		err := deserializeMsg(bytes.NewReader(data), &dest)
		require.EqualError(t, err, `decoding message data: unexpected EOF`)
	})

	t.Run("length byte +1", func(t *testing.T) {
		// first byte in the data is total stream length set by us (ie not
		// CBOR encoder). when decoding we set this as a max stream length
		// but CBOR decoder keeps track of field lengths and stops after last
		// field... should we add validation that not whole stream was consumed?
		data := append(slices.Clone(cborData), 42)
		data[0]++
		var dest testMsg
		err := deserializeMsg(bytes.NewReader(data), &dest)
		require.NoError(t, err)
		require.Equal(t, msg, dest)
	})

	t.Run("length == 0", func(t *testing.T) {
		data := slices.Clone(cborData)
		data[0] = 0
		var dest testMsg
		err := deserializeMsg(bytes.NewReader(data), &dest)
		require.EqualError(t, err, `unexpected data length zero`)
	})
}
