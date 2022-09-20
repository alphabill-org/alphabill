package proof

import (
	"crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/imt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type OnlyPrimaryTypeProvider struct {
}

func (p *OnlyPrimaryTypeProvider) IsPrimary(_ *txsystem.Transaction) bool {
	return true
}

func TestCreatePrimaryProof_OnlyPrimaryTransactions(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	b := &block.Block{
		Transactions: []*txsystem.Transaction{
			{UnitId: unitId(1)},
			{UnitId: unitId(2)},
			{UnitId: unitId(3)},
			{UnitId: unitId(4)},
			{UnitId: unitId(5)},
			{UnitId: unitId(6)},
			{UnitId: unitId(7)},
			{UnitId: unitId(8)},
			{UnitId: unitId(9)},
			{UnitId: unitId(10)},
		},
		UnicityCertificate: &certificates.UnicityCertificate{},
	}
	tx := b.Transactions[0]

	// verify hash chain
	leaves, _ := blockTreeLeaves(b.Transactions, hashAlgorithm)
	tree, _ := imt.New(leaves, hashAlgorithm)
	chain, _ := tree.GetMerklePath(tx.UnitId)
	root := imt.EvalMerklePath(chain, tx.UnitId, hashAlgorithm)
	require.Equal(t, "51DED96EFFA752C15D49E86A4C87E60AC21A09FC6AFC5E18E4650FD69531E413", fmt.Sprintf("%X", root))

	// verify primary proof for each transaction
	for i, transaction := range b.Transactions {
		p, err := CreatePrimaryProof(b, transaction.UnitId, &OnlyPrimaryTypeProvider{}, hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, ProofType_PRIM, p.ProofType)
		require.True(t, VerifyProof(transaction, p, []byte{0}, hashAlgorithm),
			"proof verification failed for tx_idx=%d", i)
	}

	// verify proof is NoTrans for non existing transaction
	p, err := CreatePrimaryProof(b, unitId(11), &OnlyPrimaryTypeProvider{}, hashAlgorithm)
	require.NoError(t, err)
	require.Equal(t, ProofType_NOTRANS, p.ProofType)
	require.True(t, VerifyProof(tx, p, []byte{0}, hashAlgorithm))
}

func unitId(num uint64) []byte {
	bytes32 := uint256.NewInt(num).Bytes32()
	return bytes32[:]
}
