package unicitytree

import (
	"crypto"
	"crypto/sha256"
	"testing"

	"github.com/alphabill-org/alphabill/tree/smt"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

var inputRecord = &types.InputRecord{
	PreviousHash: []byte{0x00},
	Hash:         []byte{0x01},
	BlockHash:    []byte{0x02},
	SummaryValue: []byte{0x03},
}

func TestNewUnicityTree(t *testing.T) {
	unicityTree, err := New(sha256.New(), []*Data{
		{
			SystemIdentifier:            1,
			InputRecord:                 inputRecord,
			SystemDescriptionRecordHash: []byte{1, 2, 3, 4},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, unicityTree)
}

func TestGetCertificate_Ok(t *testing.T) {
	key := types.SystemID(1)
	data := []*Data{
		{
			SystemIdentifier:            key,
			InputRecord:                 inputRecord,
			SystemDescriptionRecordHash: []byte{1, 2, 3, 4},
		},
	}
	unicityTree, err := New(sha256.New(), data)
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate(key)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, key, cert.SystemIdentifier)
	require.Equal(t, types.SystemIdentifierLength*8, len(cert.SiblingHashes))

	hasher := crypto.SHA256.New()
	data[0].AddToHasher(hasher)
	dataHash := hasher.Sum(nil)
	hasher.Reset()

	root, err := smt.CalculatePathRoot(cert.SiblingHashes, dataHash, key.Bytes(), crypto.SHA256)
	require.NoError(t, err)
	require.Equal(t, unicityTree.GetRootHash(), root)
	ir, err := unicityTree.GetIR(key)
	require.NoError(t, err)
	require.EqualValues(t, ir, data[0].InputRecord)
}

func TestGetCertificate_InvalidKey(t *testing.T) {
	unicityTree, err := New(sha256.New(), []*Data{
		{
			SystemIdentifier:            0x01020301,
			InputRecord:                 inputRecord,
			SystemDescriptionRecordHash: []byte{1, 2, 3, 4},
		},
	})
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate(0x0102)

	require.Nil(t, cert)
	require.EqualError(t, err, "certificate for system id 00000102 not found")
}

func TestGetCertificate_KeyNotFound(t *testing.T) {
	unicityTree, err := New(sha256.New(), []*Data{
		{
			SystemIdentifier:            0x01020301,
			InputRecord:                 inputRecord,
			SystemDescriptionRecordHash: []byte{1, 2, 3, 4},
		},
	})
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate(1)
	require.Nil(t, cert)
	require.EqualError(t, err, "certificate for system id 00000001 not found")
	ir, err := unicityTree.GetIR(1)
	require.EqualError(t, err, "ir for system id 00000001 not found")
	require.Nil(t, ir)
}
