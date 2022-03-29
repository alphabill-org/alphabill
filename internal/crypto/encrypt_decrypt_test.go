package crypto

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDataCanBeEncryptedAndDecrypted(t *testing.T) {
	data := []byte("my-secret-message")
	passphrase := "foo"

	ciphertext, err := Encrypt(passphrase, data)
	require.NoError(t, err)

	plaintext, err := Decrypt(passphrase, ciphertext)
	require.NoError(t, err)

	require.EqualValues(t, plaintext, data)
}
