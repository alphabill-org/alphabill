package cmd

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndLoadKeys(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	keys, err := GenerateKeys()
	require.NoError(t, err)
	err = keys.WriteTo(file)
	require.NoError(t, err)
	loaded, err := LoadKeys(file, false, false)
	require.NoError(t, err)
	require.Equal(t, keys.SigningPrivateKey, loaded.SigningPrivateKey)
	require.Equal(t, keys.EncryptionPrivateKey, loaded.EncryptionPrivateKey)
}

func TestLoadKeys_FileNotFound(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	keys, err := LoadKeys(file, false, false)
	require.ErrorContains(t, err, fmt.Sprintf("keys file %s not found", file))
	require.Nil(t, keys)
}

func TestLoadKeys_ForceGeneration(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	keys, err := LoadKeys(file, true, false)
	require.NoError(t, err)
	require.NotNil(t, keys)
	loaded, err := LoadKeys(file, true, false)
	require.NoError(t, err)
	require.Equal(t, keys.SigningPrivateKey, loaded.SigningPrivateKey)
	require.Equal(t, keys.EncryptionPrivateKey, loaded.EncryptionPrivateKey)
	// make sure force generation overwrites existing keys
	overwritten, err := LoadKeys(file, true, true)
	require.NoError(t, err)
	require.NotNil(t, keys)
	require.NotEqual(t, loaded.SigningPrivateKey, overwritten.SigningPrivateKey)
	require.NotEqual(t, loaded.EncryptionPrivateKey, overwritten.EncryptionPrivateKey)
	loadedOverwritten, err := LoadKeys(file, true, false)
	require.NoError(t, err)
	require.Equal(t, overwritten.SigningPrivateKey, loadedOverwritten.SigningPrivateKey)
	require.Equal(t, overwritten.EncryptionPrivateKey, loadedOverwritten.EncryptionPrivateKey)
}

func TestLoadKeys_InvalidSigningKeyAlgorithm(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	kf := &keyFile{
		SigningPrivateKey: key{
			Algorithm:  "invalid_algo",
			PrivateKey: nil,
		},
		EncryptionPrivateKey: key{},
	}
	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false, false)
	require.ErrorContains(t, err, "signing key algorithm invalid_algo is not supported")
	require.Nil(t, keys)
}

func TestLoadKeys_InvalidEncryptionKeyAlgorithm(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	signer, _ := testsig.CreateSignerAndVerifier(t)
	bytes, err := signer.MarshalPrivateKey()
	require.NoError(t, err)
	kf := &keyFile{
		SigningPrivateKey: key{
			Algorithm:  secp256k1,
			PrivateKey: bytes,
		},
		EncryptionPrivateKey: key{
			Algorithm:  "invalid_algo",
			PrivateKey: nil,
		},
	}

	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false, false)
	require.ErrorContains(t, err, "encryption key algorithm invalid_algo is not supported")
	require.Nil(t, keys)
}

func TestLoadKeys_InvalidEncryptionKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	signer, _ := testsig.CreateSignerAndVerifier(t)
	bytes, err := signer.MarshalPrivateKey()
	require.NoError(t, err)
	kf := &keyFile{
		SigningPrivateKey: key{
			Algorithm:  secp256k1,
			PrivateKey: bytes,
		},
		EncryptionPrivateKey: key{
			Algorithm:  secp256k1,
			PrivateKey: []byte{0, 0, 0},
		},
	}

	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false, false)
	require.ErrorContains(t, err, "invalid encryption key")
	require.Nil(t, keys)
}

func TestLoadKeys_InvalidSigningKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	signer, _ := testsig.CreateSignerAndVerifier(t)
	bytes, err := signer.MarshalPrivateKey()
	require.NoError(t, err)
	kf := &keyFile{
		SigningPrivateKey: key{
			Algorithm:  secp256k1,
			PrivateKey: []byte{0},
		},
		EncryptionPrivateKey: key{
			Algorithm:  secp256k1,
			PrivateKey: bytes,
		},
	}

	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false, false)
	require.ErrorContains(t, err, "invalid signing key")
	require.Nil(t, keys)
}
