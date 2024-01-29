package unicitytree

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/tree/imt"
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
	unicityTree, err := New(crypto.SHA256, []*types.UTData{
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
	data := []*types.UTData{
		{
			SystemIdentifier:            key,
			InputRecord:                 inputRecord,
			SystemDescriptionRecordHash: []byte{1, 2, 3, 4},
		},
	}
	unicityTree, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate(key)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, key, cert.SystemIdentifier)
	root := imt.IndexTreeOutput(cert.SiblingHashes, key.Bytes(), crypto.SHA256)
	require.NoError(t, err)
	require.Equal(t, unicityTree.GetRootHash(), root)
}

func TestGetCertificate_InvalidKey(t *testing.T) {
	unicityTree, err := New(crypto.SHA256, []*types.UTData{
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
	unicityTree, err := New(crypto.SHA256, []*types.UTData{
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
}
