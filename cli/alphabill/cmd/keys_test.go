package cmd

import (
	"fmt"
	"path"
	"testing"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"github.com/stretchr/testify/require"
)

const keysDir = "keys"

func TestGenerateAndLoadKeys(t *testing.T) {
	file := path.Join(setupTestDir(t, keysDir), "keys.json")
	keys, err := GenerateKeys()
	require.NoError(t, err)
	err = keys.WriteTo(file)
	require.NoError(t, err)
	loaded, err := LoadKeys(file, false)
	require.NoError(t, err)
	require.Equal(t, keys.SigningPrivateKey, loaded.SigningPrivateKey)
	require.Equal(t, keys.EncryptionPrivateKey, loaded.EncryptionPrivateKey)
}

func TestLoadKeys_FileNotFound(t *testing.T) {
	file := path.Join(setupTestDir(t, keysDir), "keys.json")
	keys, err := LoadKeys(file, false)
	require.ErrorContains(t, err, fmt.Sprintf("keys file %s not found", file))
	require.Nil(t, keys)
}

func TestLoadKeys_ForceGeneration(t *testing.T) {
	file := path.Join(setupTestDir(t, keysDir), "keys.json")
	keys, err := LoadKeys(file, true)
	require.NoError(t, err)
	require.NotNil(t, keys)
	loaded, err := LoadKeys(file, false)
	require.NoError(t, err)
	require.Equal(t, keys.SigningPrivateKey, loaded.SigningPrivateKey)
	require.Equal(t, keys.EncryptionPrivateKey, loaded.EncryptionPrivateKey)
}

func TestLoadKeys_InvalidSigningKeyAlgorithm(t *testing.T) {
	file := path.Join(setupTestDir(t, keysDir), "keys.json")
	kf := &keyFile{
		SigningPrivateKey: key{
			Algorithm:  "invalid_algo",
			PrivateKey: nil,
		},
		EncryptionPrivateKey: key{},
	}
	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false)
	require.ErrorContains(t, err, "signing key algorithm invalid_algo is not supported")
	require.Nil(t, keys)
}

func TestLoadKeys_InvalidEncryptionKeyAlgorithm(t *testing.T) {
	file := path.Join(setupTestDir(t, keysDir), "keys.json")
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

	require.NoError(t, err)
	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false)
	require.ErrorContains(t, err, "encryption key algorithm invalid_algo is not supported")
	require.Nil(t, keys)
}

func TestLoadKeys_InvalidEncryptionKey(t *testing.T) {
	file := path.Join(setupTestDir(t, keysDir), "keys.json")
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

	require.NoError(t, err)
	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false)
	require.ErrorContains(t, err, "invalid encryption key")
	require.Nil(t, keys)
}

func TestLoadKeys_InvalidSigningKey(t *testing.T) {
	file := path.Join(setupTestDir(t, keysDir), "keys.json")
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

	require.NoError(t, err)
	require.NoError(t, util.WriteJsonFile(file, kf))
	keys, err := LoadKeys(file, false)
	require.ErrorContains(t, err, "invalid signing key")
	require.Nil(t, keys)
}
