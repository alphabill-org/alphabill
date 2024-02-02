package types

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
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
		SiblingHashes:         make([][]byte, 32),
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

	t.Run("Test NewTxProof", func(t *testing.T) {
		proof, _, err := NewTxProof(block, 0, crypto.SHA256)
		require.NoError(t, err)
		require.NotNil(t, proof)
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
