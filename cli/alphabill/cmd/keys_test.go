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
	require.Equal(t, keys.SignPrivKey, loaded.SignPrivKey)
	require.Equal(t, keys.AuthPrivKey, loaded.AuthPrivKey)
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
	require.Equal(t, keys.SignPrivKey, loaded.SignPrivKey)
	require.Equal(t, keys.AuthPrivKey, loaded.AuthPrivKey)
	// make sure force generation overwrites existing keys
	overwritten, err := LoadKeys(file, true, true)
	require.NoError(t, err)
	require.NotNil(t, keys)
	require.NotEqual(t, loaded.SignPrivKey, overwritten.SignPrivKey)
	require.NotEqual(t, loaded.AuthPrivKey, overwritten.AuthPrivKey)
	loadedOverwritten, err := LoadKeys(file, true, false)
	require.NoError(t, err)
	require.Equal(t, overwritten.SignPrivKey, loadedOverwritten.SignPrivKey)
	require.Equal(t, overwritten.AuthPrivKey, loadedOverwritten.AuthPrivKey)
}

func TestLoadKeys_InvalidSigningKeyAlgorithm(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	kf := &keyFile{
		SignKey: key{
			Algorithm:  "invalid_algo",
			PrivateKey: nil,
		},
		AuthKey: key{},
	}
	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false, false)
	require.ErrorContains(t, err, "signing key algorithm invalid_algo is not supported")
	require.Nil(t, keys)
}

func TestLoadKeys_InvalidAuthenticationKeyAlgorithm(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	signer, _ := testsig.CreateSignerAndVerifier(t)
	bytes, err := signer.MarshalPrivateKey()
	require.NoError(t, err)
	kf := &keyFile{
		SignKey: key{
			Algorithm:  secp256k1,
			PrivateKey: bytes,
		},
		AuthKey: key{
			Algorithm:  "invalid_algo",
			PrivateKey: nil,
		},
	}

	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false, false)
	require.ErrorContains(t, err, "authentication key algorithm invalid_algo is not supported")
	require.Nil(t, keys)
}

func TestLoadKeys_InvalidAuthenticationKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	signer, _ := testsig.CreateSignerAndVerifier(t)
	bytes, err := signer.MarshalPrivateKey()
	require.NoError(t, err)
	kf := &keyFile{
		SignKey: key{
			Algorithm:  secp256k1,
			PrivateKey: bytes,
		},
		AuthKey: key{
			Algorithm:  secp256k1,
			PrivateKey: []byte{0, 0, 0},
		},
	}

	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false, false)
	require.ErrorContains(t, err, "invalid authentication key")
	require.Nil(t, keys)
}

func TestLoadKeys_InvalidSigningKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.json")
	signer, _ := testsig.CreateSignerAndVerifier(t)
	bytes, err := signer.MarshalPrivateKey()
	require.NoError(t, err)
	kf := &keyFile{
		SignKey: key{
			Algorithm:  secp256k1,
			PrivateKey: []byte{0},
		},
		AuthKey: key{
			Algorithm:  secp256k1,
			PrivateKey: bytes,
		},
	}

	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false, false)
	require.ErrorContains(t, err, "invalid signing key")
	require.Nil(t, keys)
}
