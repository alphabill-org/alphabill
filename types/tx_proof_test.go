package types

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/tree/imt"
	"github.com/stretchr/testify/require"
)

func TestTxProofFunctions(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            NewTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign("test", signer)
	require.NoError(t, err)
	txOrder := createTxOrder(t)

	uc := &UnicityCertificate{InputRecord: ir, UnicitySeal: seal, UnicityTreeCertificate: &UnicityTreeCertificate{
		SystemIdentifier:      systemID,
		SiblingHashes:         []*imt.PathItem{{Key: identifier.Bytes(), Hash: []byte{1, 2, 3}}},
		SystemDescriptionHash: zeroHash,
	}}
	block := &Block{
		Header: &Header{
			SystemID:          systemID,
			ShardID:           test.RandomBytes(33),
			ProposerID:        "proposer123",
			PreviousBlockHash: test.RandomBytes(33),
		},
		Transactions:       []*TransactionRecord{{TransactionOrder: txOrder}},
		UnicityCertificate: uc,
	}

	t.Run("Test NewTxProof OK", func(t *testing.T) {
		proof, record, err := NewTxProof(block, 0, crypto.SHA256)
		require.NoError(t, err)
		require.Equal(t, block.HeaderHash(crypto.SHA256), proof.BlockHeaderHash)
		require.Empty(t, proof.Chain)
		require.Equal(t, uc, proof.UnicityCertificate)
		require.Len(t, block.Transactions, 1)
		require.Equal(t, block.Transactions[0], record)
	})

	t.Run("Test NewTxProof nil block", func(t *testing.T) {
		proof, record, err := NewTxProof(nil, 0, crypto.SHA256)
		require.Nil(t, proof)
		require.Nil(t, record)
		require.ErrorContains(t, err, "block is nil")
	})

	t.Run("Test NewTxProof empty block", func(t *testing.T) {
		proof, record, err := NewTxProof(&Block{}, 0, crypto.SHA256)
		require.Nil(t, proof)
		require.Nil(t, record)
		require.ErrorContains(t, err, "invalid tx index")
	})

	t.Run("Test VerifyTxProof ", func(t *testing.T) {
		proof, txRecord, err := NewTxProof(block, 0, crypto.SHA256)
		require.NoError(t, err)

		trustBase := make(map[string]abcrypto.Verifier)
		trustBase["test"] = verifier

		err = VerifyTxProof(proof, txRecord, trustBase, crypto.SHA256)
		require.ErrorContains(t, err, "invalid unicity certificate")
	})
}
