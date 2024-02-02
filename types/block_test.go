package types

import (
	"crypto"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestBlockFunctions(t *testing.T) {
	signer, _ := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            NewTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign("test", signer)
	require.NoError(t, err)

	uc := &UnicityCertificate{InputRecord: ir, UnicitySeal: seal}
	block := &Block{
		Header: &Header{
			SystemID:          systemID,
			ShardID:           test.RandomBytes(33),
			ProposerID:        "proposer123",
			PreviousBlockHash: test.RandomBytes(33),
		},
		Transactions:       []*TransactionRecord{{TransactionOrder: createTxOrder(t)}},
		UnicityCertificate: uc,
	}
	emptyBlock := &Block{}

	t.Run("Test Hash", func(t *testing.T) {
		hash, err := block.Hash(crypto.SHA256)
		require.NoError(t, err)
		require.NotNil(t, hash)

		hash2, err := emptyBlock.Hash(crypto.SHA256)
		require.NoError(t, err)
		require.Equal(t, make([]byte, 32), hash2)
	})

	t.Run("Test HeaderHash", func(t *testing.T) {
		require.NotNil(t, block.HeaderHash(crypto.SHA256))
	})

	t.Run("Test GetRoundNumber", func(t *testing.T) {
		require.Equal(t, seal.RootChainRoundNumber, block.GetRoundNumber())
	})

	t.Run("Test IsValid", func(t *testing.T) {
		err := block.IsValid(func(uc *UnicityCertificate) error {
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("Test GetProposerID", func(t *testing.T) {
		require.Equal(t, "proposer123", block.GetProposerID())
		require.Empty(t, emptyBlock.GetProposerID())
	})

	t.Run("Test SystemID", func(t *testing.T) {
		require.Equal(t, systemID, block.SystemID())
		require.Zero(t, emptyBlock.SystemID())
	})
}
