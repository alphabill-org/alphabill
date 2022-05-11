package state

import (
	"crypto"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/holiman/uint256"
)

func TestExtractCertificate_Ok(t *testing.T) {
	at, err := New(defaultConfig())
	require.NoError(t, err)
	keys := []*uint256.Int{key10, key30, key20, key25, key31}

	// add nodes
	for _, key := range keys {
		at.setNode(key, newNodeContent(int(key.Uint64())))
	}

	// calculate root
	root := at.GetRootHash()

	// extract authentication paths
	for _, key := range keys {
		cert, err := at.ExtractCertificate(key)
		require.NoError(t, err)
		require.NotNil(t, cert)
		z := calculateZ(key, at)
		certRoot, summary, err := cert.CompTreeCert(key, z, Uint64SummaryValue(int(key.Uint64())), crypto.SHA256)
		require.NoError(t, err)
		require.Equal(t, root, certRoot)
		require.Equal(t, Uint64SummaryValue(116), summary)
	}
}

func TestExtractCertificate_RootIsNil(t *testing.T) {
	at, _ := New(defaultConfig())
	_, err := at.ExtractCertificate(key10)
	require.ErrorIs(t, ErrRootNodeIsNil, err)
}

func TestExtractCertificate_IdIsNil(t *testing.T) {
	at, _ := New(defaultConfig())
	_, err := at.ExtractCertificate(nil)
	require.ErrorIs(t, ErrIdIsNil, err)
}

func TestExtractCertificate_RootNotComputed(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key10, newNodeContent(int(key10.Uint64())))
	_, err := at.ExtractCertificate(key10)
	require.ErrorIs(t, ErrRootNotCalculated, err)
}

func TestExtractCertificate_ItemNotFound(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key10, newNodeContent(int(key10.Uint64())))
	at.GetRootHash()
	_, err := at.ExtractCertificate(key11)
	require.ErrorContains(t, err, fmt.Sprintf(errStrItemDoesntExist, 11))
}

func TestCompTreeCert_IdIsNil(t *testing.T) {
	c := &certificate{}
	_, _, err := c.CompTreeCert(nil, []byte{}, Uint64SummaryValue(10), crypto.SHA256)
	require.ErrorIs(t, ErrIdIsNil, err)
}

func TestCompTreeCert_ZIsNil(t *testing.T) {
	c := &certificate{}
	_, _, err := c.CompTreeCert(uint256.NewInt(1), nil, Uint64SummaryValue(10), crypto.SHA256)
	require.ErrorIs(t, ErrZIsNil, err)
}

func TestCompTreeCert_SummaryValueIsNil(t *testing.T) {
	c := &certificate{}
	_, _, err := c.CompTreeCert(uint256.NewInt(1), []byte{}, nil, crypto.SHA256)
	require.ErrorIs(t, ErrSummaryValueIsNil, err)
}

func calculateZ(key *uint256.Int, at *rmaTree) []byte {
	content, _ := at.get(key)
	idBytes := key.Bytes32()
	hasher := crypto.SHA256.New()
	hasher.Write(idBytes[:])
	hasher.Write(content.Bearer)
	content.Data.AddToHasher(hasher)
	hashSub1 := hasher.Sum(nil)
	hasher.Reset()
	hasher.Write(content.StateHash)
	hasher.Write(hashSub1)
	z := hasher.Sum(nil)
	hasher.Reset()
	return z
}

func defaultConfig() *Config {
	return &Config{HashAlgorithm: crypto.SHA256, TrustBase: []string{"0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"}}
}
