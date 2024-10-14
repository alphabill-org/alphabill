package unicitytree

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/tree/imt"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

var inputRecord = &types.InputRecord{Version: 1,
	PreviousHash: []byte{0x00},
	Hash:         []byte{0x01},
	BlockHash:    []byte{0x02},
	SummaryValue: []byte{0x03},
}

func TestNewUnicityTree(t *testing.T) {
	unicityTree, err := New(crypto.SHA256, []*types.UnicityTreeData{
		{
			SystemIdentifier:         1,
			InputRecord:              inputRecord,
			PartitionDescriptionHash: []byte{1, 2, 3, 4},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, unicityTree)
}

func TestGetCertificate_Ok(t *testing.T) {
	key1 := types.SystemID(1)
	key2 := types.SystemID(2)
	data := []*types.UnicityTreeData{
		{
			SystemIdentifier:         key2,
			InputRecord:              inputRecord,
			PartitionDescriptionHash: []byte{3, 4, 5, 6},
		},
		{
			SystemIdentifier:         key1,
			InputRecord:              inputRecord,
			PartitionDescriptionHash: []byte{1, 2, 3, 4},
		},
	}
	unicityTree, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate(key1)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, key1, cert.SystemIdentifier)

	var hashSteps []*imt.PathItem
	hashSteps = append(hashSteps, cert.FirstHashStep(inputRecord, crypto.SHA256))
	hashSteps = append(hashSteps, cert.HashSteps...)

	root := imt.IndexTreeOutput(hashSteps, key1.Bytes(), crypto.SHA256)
	require.NoError(t, err)
	require.Equal(t, unicityTree.GetRootHash(), root)
	// system id 0 is illegal
	cert, err = unicityTree.GetCertificate(types.SystemID(0))
	require.EqualError(t, err, "partition ID is unassigned")
	require.Nil(t, cert)
}

func TestGetCertificate_InvalidKey(t *testing.T) {
	unicityTree, err := New(crypto.SHA256, []*types.UnicityTreeData{
		{
			SystemIdentifier:         0x01020301,
			InputRecord:              inputRecord,
			PartitionDescriptionHash: []byte{1, 2, 3, 4},
		},
	})
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate(0x0102)

	require.Nil(t, cert)
	require.EqualError(t, err, "certificate for system id 00000102 not found")
}

func TestGetCertificate_KeyNotFound(t *testing.T) {
	unicityTree, err := New(crypto.SHA256, []*types.UnicityTreeData{
		{
			SystemIdentifier:         0x01020301,
			InputRecord:              inputRecord,
			PartitionDescriptionHash: []byte{1, 2, 3, 4},
		},
	})
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate(1)
	require.Nil(t, cert)
	require.EqualError(t, err, "certificate for system id 00000001 not found")
}
