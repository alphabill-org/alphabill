package types

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func createBlock(t *testing.T, id string, signer abcrypto.Signer) *Block {
	sdrs := &SystemDescriptionRecord{
		SystemIdentifier: systemID,
		T2Timeout:        2500,
	}
	inputRecord := &InputRecord{
		PreviousHash:    []byte{0, 0, 1},
		Hash:            []byte{0, 0, 2},
		SummaryValue:    []byte{0, 0, 4},
		RoundNumber:     1,
		SumOfEarnedFees: 2,
	}
	txr1 := createTransactionRecord(createTxOrder(t), 1)
	txr2 := createTransactionRecord(createTxOrder(t), 1)
	block := &Block{
		Header: &Header{
			SystemID:          systemID,
			ProposerID:        "proposer123",
			PreviousBlockHash: []byte{1, 2, 3},
		},
		Transactions: []*TransactionRecord{txr1, txr2},
		UnicityCertificate: &UnicityCertificate{
			InputRecord: inputRecord,
		},
	}
	// calculate block hash
	blockhash, err := block.Hash(crypto.SHA256)
	require.NoError(t, err)
	inputRecord.BlockHash = blockhash
	block.UnicityCertificate = createUnicityCertificate(t, id, signer, inputRecord, sdrs)
	return block
}

func TestTxProofFunctions(t *testing.T) {

	t.Run("Test NewTxProof OK", func(t *testing.T) {
		signer, _ := testsig.CreateSignerAndVerifier(t)
		block := createBlock(t, "test", signer)
		proof, record, err := NewTxProof(block, 0, crypto.SHA256)
		require.NoError(t, err)
		require.Equal(t, block.HeaderHash(crypto.SHA256), proof.BlockHeaderHash)
		require.Len(t, proof.Chain, 1)
		require.Equal(t, block.UnicityCertificate, proof.UnicityCertificate)
		require.Len(t, block.Transactions, 2)
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

	t.Run("Test VerifyTxProof ok", func(t *testing.T) {
		signer, verifier := testsig.CreateSignerAndVerifier(t)
		block := createBlock(t, "test", signer)
		proof, txRecord, err := NewTxProof(block, 0, crypto.SHA256)
		require.NoError(t, err)

		trustBase := make(map[string]abcrypto.Verifier)
		trustBase["test"] = verifier
		require.NoError(t, VerifyTxProof(proof, txRecord, trustBase, crypto.SHA256))
	})

	t.Run("Test proof is nil", func(t *testing.T) {
		trustBase := make(map[string]abcrypto.Verifier)
		require.EqualError(t, VerifyTxProof(nil, nil, trustBase, crypto.SHA256), "tx proof is nil")
	})

	t.Run("Test tx record is nil", func(t *testing.T) {
		trustBase := make(map[string]abcrypto.Verifier)
		proof := &TxProof{}
		require.EqualError(t, VerifyTxProof(proof, nil, trustBase, crypto.SHA256), "tx record is nil")
	})

	t.Run("Test tx record is nil", func(t *testing.T) {
		trustBase := make(map[string]abcrypto.Verifier)
		proof := &TxProof{}
		txr := &TransactionRecord{}
		require.EqualError(t, VerifyTxProof(proof, txr, trustBase, crypto.SHA256), "tx order is nil")
	})

	t.Run("Test VerifyTxProof error, invalid system id", func(t *testing.T) {
		signer, verifier := testsig.CreateSignerAndVerifier(t)
		block := createBlock(t, "test", signer)
		proof, txRecord, err := NewTxProof(block, 0, crypto.SHA256)
		require.NoError(t, err)

		trustBase := make(map[string]abcrypto.Verifier)
		trustBase["test"] = verifier
		proof.UnicityCertificate.UnicityTreeCertificate.SystemIdentifier = SystemID(1)
		require.EqualError(t, VerifyTxProof(proof, txRecord, trustBase, crypto.SHA256),
			"invalid unicity certificate: unicity certificate validation failed: unicity tree certificate validation failed: invalid system identifier: expected 01000001, got 00000001")
	})

	t.Run("Test VerifyTxProof error, invalid block hash", func(t *testing.T) {
		signer, verifier := testsig.CreateSignerAndVerifier(t)
		block := createBlock(t, "test", signer)
		proof, txRecord, err := NewTxProof(block, 0, crypto.SHA256)
		require.NoError(t, err)

		trustBase := make(map[string]abcrypto.Verifier)
		trustBase["test"] = verifier
		proof.BlockHeaderHash = make([]byte, 32)
		require.EqualError(t, VerifyTxProof(proof, txRecord, trustBase, crypto.SHA256), "proof block hash does not match to block hash in unicity certificate")
	})
}
