package unicitytree

import (
	"crypto/sha256"
	"hash"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"github.com/stretchr/testify/require"
)

var inputRecord = InputRecord{
	PreviousHash: []byte{0x00},
	Hash:         []byte{0x01},
	BlockHash:    []byte{0x02},
	SummaryValue: Uint64SummaryValue(1),
}

func TestNewUnicityTree(t *testing.T) {
	unicityTree, err := New(sha256.New(), []*Data{{systemIdentifier: []byte{0x00, 0x00, 0x00, 0x01}, inputRecord: inputRecord}})
	require.NoError(t, err)
	require.NotNil(t, unicityTree)
}

func TestGetCertificate_Ok(t *testing.T) {
	key := []byte{0x00, 0x00, 0x00, 0x01}
	unicityTree, err := New(sha256.New(), []*Data{{systemIdentifier: key, inputRecord: inputRecord}})
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate(key)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, key, cert.systemIdentifier)
	require.Equal(t, systemIdentifierLength*8, len(cert.siblingHashes))
}

func TestGetCertificate_InvalidKey(t *testing.T) {
	unicityTree, err := New(sha256.New(), []*Data{{systemIdentifier: []byte{0x00, 0x00, 0x00, 0x01}, inputRecord: inputRecord}})
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate([]byte{0x00, 0x00})

	require.Nil(t, cert)
	require.ErrorIs(t, err, ErrInvalidSystemIdentifierLength)
}

type Uint64SummaryValue uint64

func (t Uint64SummaryValue) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(uint64(t)))
}

func (t Uint64SummaryValue) Concatenate(left, right state.SummaryValue) state.SummaryValue {
	var s, l, r uint64
	s = uint64(t)
	lSum, ok := left.(Uint64SummaryValue)
	if ok {
		l = uint64(lSum)
	}
	rSum, ok := right.(Uint64SummaryValue)
	if ok {
		r = uint64(rSum)
	}
	return Uint64SummaryValue(s + l + r)
}
