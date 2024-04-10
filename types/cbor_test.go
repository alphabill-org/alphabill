package types

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

type CustomData struct {
	Name  string
	Value int
}

func TestCborHandler_Marshal(t *testing.T) {
	var (
		validInput = CustomData{Name: "foo", Value: 10}
		validCbor  = []byte{0xa2, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x63, 0x66, 0x6f, 0x6f, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65, 0xa}
	)

	cases := []struct {
		name     string
		input    any
		expected []byte
		wantErr  string
	}{
		{
			name:     "Marshal valid input",
			input:    validInput,
			expected: validCbor,
		},
		{
			name:     "Marshal invalid data input",
			input:    complex(20, 10),
			expected: nil,
			wantErr:  "cbor: unsupported type: complex128",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Cbor.Marshal(tc.input)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			}
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestCborHandler_Unmarshal(t *testing.T) {
	var (
		validInput  = CustomData{Name: "foo", Value: 20}
		validCbor   = []byte{0xa2, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x63, 0x66, 0x6f, 0x6f, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x14}
		invalidCbor = []byte{0xa2, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x63, 0x66, 0x6f, 0x6f, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x18} // missing final value
	)

	t.Run("Unmarshal valid input", func(t *testing.T) {
		var got CustomData
		err := Cbor.Unmarshal(validCbor, &got)
		require.NoError(t, err)
		require.Equal(t, validInput, got)
	})

	t.Run("Unmarshal nil and empty input", func(t *testing.T) {
		var got CustomData
		err := Cbor.Unmarshal(nil, &got)
		require.ErrorContains(t, err, "EOF")
		require.Equal(t, CustomData{}, got)

		err = Cbor.Unmarshal([]byte{}, &got)
		require.ErrorContains(t, err, "EOF")
		require.Equal(t, CustomData{}, got)
	})

	t.Run("Unmarshal invalid input data", func(t *testing.T) {
		var got CustomData
		err := Cbor.Unmarshal([]byte{5}, &got)
		require.ErrorContains(t, err, "cbor: cannot unmarshal positive integer into Go value of type types.CustomData")
		require.Equal(t, CustomData{}, got)

		err = Cbor.Unmarshal(invalidCbor, &got)
		require.ErrorContains(t, err, "unexpected EOF")
		require.Equal(t, CustomData{}, got)
	})

	t.Run("Unmarshal non-pointer", func(t *testing.T) {
		var got CustomData
		err := Cbor.Unmarshal(validCbor, got)
		require.ErrorContains(t, err, "cbor: Unmarshal(non-pointer types.CustomData)")
		require.Equal(t, CustomData{}, got)
	})

	t.Run("Unmarshal wrong type", func(t *testing.T) {
		var got Bytes
		err := Cbor.Unmarshal(validCbor, &got)
		require.ErrorContains(t, err, "cbor: cannot unmarshal map into Go value of type types.Bytes")
		require.Nil(t, got)
	})
}

func TestCborHandler_Encoding(t *testing.T) {
	var (
		validInput    = CustomData{Name: "foo", Value: 30}
		validCbor     = []byte{0xa2, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x63, 0x66, 0x6f, 0x6f, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x1e}
		emptyDataCbor = []byte{0xa2, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x60, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x0}
	)

	cases := []struct {
		name     string
		input    any
		expected []byte
		wantErr  string
	}{
		{
			name:     "Valid encoding",
			input:    validInput,
			expected: validCbor,
		},
		{
			name:     "Empty data encoding",
			input:    CustomData{},
			expected: emptyDataCbor,
		},
		{
			name:     "Nil data encoding",
			input:    nil,
			expected: []byte{0xf6},
		},
		{
			name:    "Invalid data encoding",
			input:   complex(20, 10),
			wantErr: "cbor: unsupported type: complex128",
		},
	}

	for _, tc := range cases {
		t.Run("Cbor.Encode: "+tc.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			err := Cbor.Encode(buf, tc.input)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			}
			require.Equal(t, tc.expected, buf.Bytes())
		})

		t.Run("Cbor.GetEncoder: "+tc.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			enc, err := Cbor.GetEncoder(buf)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			err = enc.Encode(tc.input)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			}
			require.Equal(t, tc.expected, buf.Bytes())
		})
	}
}

func TestCborHandler_Decoding(t *testing.T) {
	var (
		validInput    = CustomData{Name: "foo", Value: 40}
		validCbor     = []byte{0xa2, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x63, 0x66, 0x6f, 0x6f, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x28}
		invalidCbor   = []byte{0xa2, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x63, 0x66, 0x6f, 0x6f, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x18} // missing final value
		emptyDataCbor = []byte{0xa2, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x60, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x0}
	)

	cases := []struct {
		name     string
		input    []byte
		expected any
		wantErr  string
	}{
		{
			name:     "Valid decoding",
			input:    validCbor,
			expected: validInput,
		},
		{
			name:     "Empty data decoding",
			input:    emptyDataCbor,
			expected: CustomData{},
		},
		{
			name:     "Nil data decoding",
			input:    nil,
			expected: CustomData{},
			wantErr:  "EOF",
		},
		{
			name:     "Invalid decoding",
			input:    invalidCbor,
			expected: CustomData{},
			wantErr:  "unexpected EOF",
		},
		{
			name:     "Invalid decoding",
			input:    []byte{5},
			expected: CustomData{},
			wantErr:  "cbor: cannot unmarshal positive integer into Go value of type types.CustomData",
		},
	}

	for _, tc := range cases {
		t.Run("Cbor.GetDecoder: "+tc.name, func(t *testing.T) {
			buf := bytes.NewReader(tc.input)
			dec := Cbor.GetDecoder(buf)
			var got CustomData
			err := dec.Decode(&got)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expected, got)
		})

		t.Run("Cbor.Decode: "+tc.name, func(t *testing.T) {
			buf := bytes.NewReader(tc.input)
			var got CustomData
			err := Cbor.Decode(buf, &got)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expected, got)
		})
	}
}

func Test_RawCBOR(t *testing.T) {
	t.Run("MarshalCBOR empty input returns CBOR nil marker", func(t *testing.T) {
		// input is nil slice
		var r RawCBOR = nil
		b, err := r.MarshalCBOR()
		require.NoError(t, err)
		require.Equal(t, cborNil, b)

		// input is zero length slice
		r = make(RawCBOR, 0)
		b, err = r.MarshalCBOR()
		require.NoError(t, err)
		require.Equal(t, cborNil, b)
	})

	t.Run("UnmarshalCBOR on nil pointer", func(t *testing.T) {
		var r *RawCBOR = nil
		err := r.UnmarshalCBOR([]byte{1, 2, 3, 4})
		require.EqualError(t, err, `UnmarshalCBOR on nil pointer`)
		require.Empty(t, r)
	})

	t.Run("UnmarshalCBOR CBOR nil marker results in empty slice", func(t *testing.T) {
		// destination slice is empty
		var r RawCBOR
		require.NoError(t, r.UnmarshalCBOR(cborNil))
		require.Empty(t, r)

		// the destination must be reset when it is not empty initially
		r = RawCBOR{6, 6, 6}
		require.NoError(t, r.UnmarshalCBOR(cborNil))
		require.Empty(t, r)
	})

	t.Run("UnmarshalCBOR", func(t *testing.T) {
		// content of non-empty destination is replaced with new data
		data := []byte{9, 8, 7}
		r := RawCBOR{6, 6, 6, 6}
		require.NoError(t, r.UnmarshalCBOR(data))
		require.EqualValues(t, data, r)
		require.Equal(t, 4, cap(r))
	})

	t.Run("MarshalCBOR -> UnmarshalCBOR roundtrip", func(t *testing.T) {
		data := RawCBOR{5, 5, 5}
		buf, err := data.MarshalCBOR()
		require.NoError(t, err)

		var d RawCBOR
		require.NoError(t, d.UnmarshalCBOR(buf))
		require.Equal(t, data, d)
	})
}
