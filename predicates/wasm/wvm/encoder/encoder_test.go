package encoder

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_TVEnc_error(t *testing.T) {
	// test the "error behaviour" of the TVEnc

	t.Run("setErr", func(t *testing.T) {
		// the setErr method is supposed to record the first non-nil
		// error sent as param
		enc := &TVEnc{}
		require.Nil(t, enc.err, "initially nil")

		// calling with nil when current value is nil
		require.Nil(t, enc.setErr(nil))
		require.Nil(t, enc.err)

		// setting error
		expErr := errors.New("not good")
		require.ErrorIs(t, enc.setErr(expErr), expErr)
		require.ErrorIs(t, enc.err, expErr)

		// calling with nil doesn't reset the current error
		require.Nil(t, enc.setErr(nil))
		require.ErrorIs(t, enc.err, expErr)

		// calling with new non-nil error keeps the first one
		newErr := errors.New("and now this")
		require.ErrorIs(t, enc.setErr(newErr), newErr) // call returns the argument...
		require.ErrorIs(t, enc.err, expErr)            // ...but field kept it's value

		_, err := enc.Bytes()
		require.ErrorIs(t, err, expErr, "Bytes is expected to return the error")
	})

	t.Run("Encode", func(t *testing.T) {
		// Encode is the only method which can set the error
		enc := &TVEnc{}
		err := enc.Encode(struct{ foo string }{"anon"})
		require.EqualError(t, err, `unsupported type: struct { foo string }`)

		// successful Encode call doesn't reset the error
		require.NoError(t, enc.Encode("foobar"))

		_, err = enc.Bytes()
		require.EqualError(t, err, `unsupported type: struct { foo string }`)
	})

	t.Run("Encode array", func(t *testing.T) {
		// Encode is the only method which can set the error, array encoding is recursive
		enc := &TVEnc{}
		err := enc.Encode([]any{struct{ foo string }{"anon"}})
		require.EqualError(t, err, `encoding array item: unsupported type: struct { foo string }`)

		// successful Encode call doesn't reset the error
		require.NoError(t, enc.Encode("foobar"))

		_, err = enc.Bytes()
		require.EqualError(t, err, `unsupported type: struct { foo string }`)
	})
}
