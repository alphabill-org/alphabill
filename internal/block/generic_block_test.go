package block

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

func TestGenericBlockHash_emptyBlock(t *testing.T) {
	b := &GenericBlock{}
	hashAlgorithm := gocrypto.SHA256
	hash, err := b.Hash(hashAlgorithm)
	require.NoError(t, err)
	require.Equal(t, make([]byte, hashAlgorithm.Size()), hash)
}

func TestGenericBlockHash_nonemptyBlock(t *testing.T) {
	b := &GenericBlock{Transactions: []txsystem.GenericTransaction{createPrimaryTx(1)}}
	hashAlgorithm := gocrypto.SHA256
	hash, err := b.Hash(hashAlgorithm)
	require.NoError(t, err)
	require.NotEqual(t, make([]byte, hashAlgorithm.Size()), hash)
}
