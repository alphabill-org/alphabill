package proof

import (
	"crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/omt"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type OnlySecondaryTx struct {
	txsystem.GenericTransaction
}

func (g *OnlySecondaryTx) IsPrimary() bool {
	return false
}

func TestPrimProof_OnlyPrimTxsInBlock(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	b := &block.GenericBlock{}
	for num := uint64(1); num <= 10; num++ {
		b.Transactions = append(b.Transactions, createPrimaryTx(num))
	}
	uc, verifier := createUC(t, b, hashAlgorithm)
	b.UnicityCertificate = uc

	// verify primary proof for each transaction
	for i, transaction := range b.Transactions {
		verifyHashChain(t, b, transaction, hashAlgorithm)

		p, err := NewPrimaryProof(b, transaction.UnitID(), hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, ProofType_PRIM, p.ProofType)
		require.Nil(t, p.Verify(transaction, verifier, hashAlgorithm),
			"proof verification failed for tx_idx=%d", i)
	}

	// verify proof is NoTrans for non existing transaction
	tx := createPrimaryTx(11)
	p, err := NewPrimaryProof(b, uint256.NewInt(11), hashAlgorithm)
	require.NoError(t, err)
	require.Equal(t, ProofType_NOTRANS, p.ProofType)
	require.Nil(t, p.Verify(tx, verifier, hashAlgorithm))
}

func TestSecProof_OnlySecTxsInBlock(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	b := &block.GenericBlock{}
	for i := 0; i < 10; i++ {
		b.Transactions = append(b.Transactions, createSecondaryTx(1))
	}
	uc, verifier := createUC(t, b, hashAlgorithm)
	b.UnicityCertificate = uc

	// verify secondary proof for each transaction
	for i, tx := range b.Transactions {
		p, err := NewSecondaryProof(b, tx.UnitID(), i, hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, ProofType_SEC, p.ProofType)
		require.Nil(t, p.Verify(tx, verifier, hashAlgorithm),
			"proof verification failed for tx_idx=%d", i)

		nonExistentTxInBlock := createSecondaryTx(2)
		require.Error(t, p.Verify(nonExistentTxInBlock, verifier, hashAlgorithm),
			"proof verification should fail for non existent tx in a block")
	}
}

func createPrimaryTx(unitid uint64) txsystem.GenericTransaction {
	transferOrder := newTransferOrder(test.RandomBytes(32), 777, test.RandomBytes(32))
	transaction := newTransaction(unitId(unitid), test.RandomBytes(32), 555, transferOrder)
	tx, _ := money.NewMoneyTx([]byte{0, 0, 0, 0}, transaction)
	return tx
}

func createSecondaryTx(unitid uint64) *OnlySecondaryTx {
	tx := createPrimaryTx(unitid)
	return &OnlySecondaryTx{tx}
}

func createUC(t *testing.T, b *block.GenericBlock, hashAlgorithm crypto.Hash) (*certificates.UnicityCertificate, abcrypto.Verifier) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	blockhash, _ := b.Hash(hashAlgorithm)
	ir := &certificates.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		BlockHash:    blockhash,
		SummaryValue: make([]byte, 32),
	}
	uc := testcertificates.CreateUnicityCertificate(
		t,
		signer,
		ir,
		&genesis.SystemDescriptionRecord{SystemIdentifier: make([]byte, 4)},
		1,
		make([]byte, 32),
	)
	return uc, verifier
}

func verifyHashChain(t *testing.T, b *block.GenericBlock, tx txsystem.GenericTransaction, hashAlgorithm crypto.Hash) {
	unitIdBytes := tx.UnitID().Bytes32()
	leaves, _ := omt.BlockTreeLeaves(b.Transactions, hashAlgorithm)
	chain, _ := treeChain(tx.UnitID(), leaves, hashAlgorithm)
	root := omt.EvalMerklePath(chain, unitIdBytes[:], hashAlgorithm)
	require.Equal(t, "CB640C13D144809E963F82D393C88C3636E8672BE1EB400791649EDDF64DB578", fmt.Sprintf("%X", root))
}

func unitId(num uint64) []byte {
	bytes32 := uint256.NewInt(num).Bytes32()
	return bytes32[:]
}

func newTransaction(id, ownerProof []byte, timeout uint64, attr proto.Message) *txsystem.Transaction {
	tx := &txsystem.Transaction{
		SystemId:              []byte{0, 0, 0, 0},
		UnitId:                id,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
	_ = anypb.MarshalFrom(tx.TransactionAttributes, attr, proto.MarshalOptions{})
	return tx
}

func newTransferOrder(newBearer []byte, targetValue uint64, backlink []byte) *money.TransferOrder {
	return &money.TransferOrder{
		NewBearer:   newBearer,
		TargetValue: targetValue,
		Backlink:    backlink,
	}
}
