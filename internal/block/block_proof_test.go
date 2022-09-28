package block

import (
	"crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/omt"
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

func TestProofTypePrim(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	b := &GenericBlock{}
	for num := uint64(1); num <= 10; num++ {
		b.Transactions = append(b.Transactions, createPrimaryTx(num))
	}
	uc, verifier := createUC(t, b, hashAlgorithm)
	b.UnicityCertificate = uc

	// verify primary proof for each transaction
	for i, tx := range b.Transactions {
		verifyHashChain(t, b, tx, hashAlgorithm)

		p, err := NewPrimaryProof(b, tx.UnitID(), hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, ProofType_PRIM, p.ProofType)
		require.Nil(t, p.Verify(tx, verifier, hashAlgorithm),
			"proof verification failed for tx_idx=%d", i)

		// test proof does not verify for wrong transaction
		tx = createPrimaryTx(11)
		err = p.Verify(tx, verifier, hashAlgorithm)
		require.ErrorIs(t, err, ErrProofVerificationFailed)
	}
}

func TestProofTypeSec(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	b := &GenericBlock{}
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

		// test proof does not verify invalid tx
		nonExistentTxInBlock := createSecondaryTx(2)
		require.ErrorIs(t, p.Verify(nonExistentTxInBlock, verifier, hashAlgorithm), ErrProofVerificationFailed,
			"proof verification should fail for non existent tx in a block")
	}
}

func TestProofTypeOnlySec(t *testing.T) {
	// create block with secondary transactions for a given unit
	hashAlgorithm := crypto.SHA256
	b := &GenericBlock{
		Transactions: []txsystem.GenericTransaction{
			createSecondaryTx(1),
			createSecondaryTx(1),
			createSecondaryTx(1),
		},
	}
	uc, verifier := createUC(t, b, hashAlgorithm)
	b.UnicityCertificate = uc

	for i, tx := range b.Transactions {
		p, err := NewPrimaryProof(b, tx.UnitID(), hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, ProofType_ONLYSEC, p.ProofType)
		require.Nil(t, p.Verify(tx, verifier, hashAlgorithm),
			"proof verification failed for tx_idx=%d", i)

		// test proof does not verify for wrong transaction
		tx = createPrimaryTx(2)
		err = p.Verify(tx, verifier, hashAlgorithm)
		require.ErrorIs(t, err, ErrProofVerificationFailed)
	}
}

func TestProofTypeNoTrans(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	b := &GenericBlock{}
	b.Transactions = []txsystem.GenericTransaction{
		createPrimaryTx(1),
	}
	uc, verifier := createUC(t, b, hashAlgorithm)
	b.UnicityCertificate = uc

	tx := createPrimaryTx(11)
	p, err := NewPrimaryProof(b, uint256.NewInt(11), hashAlgorithm)
	require.NoError(t, err)
	require.Equal(t, ProofType_NOTRANS, p.ProofType)
	require.Nil(t, p.Verify(tx, verifier, hashAlgorithm))
}

func TestProofTypeEmptyBlock(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	b := &GenericBlock{}
	uc, verifier := createUC(t, b, hashAlgorithm)
	b.UnicityCertificate = uc

	p, err := NewPrimaryProof(b, uint256.NewInt(1), hashAlgorithm)
	require.NoError(t, err)
	require.Equal(t, ProofType_EMPTYBLOCK, p.ProofType)
	require.Nil(t, p.Verify(createPrimaryTx(1), verifier, hashAlgorithm))
}

func createPrimaryTx(unitid uint64) txsystem.GenericTransaction {
	transferOrder := newTransferOrder(make([]byte, 32), 777, make([]byte, 32))
	transaction := newTransaction(unitId(unitid), make([]byte, 32), 555, transferOrder)
	tx, _ := money.NewMoneyTx([]byte{0, 0, 0, 0}, transaction)
	return tx
}

func createSecondaryTx(unitid uint64) *OnlySecondaryTx {
	tx := createPrimaryTx(unitid)
	return &OnlySecondaryTx{tx}
}

func createUC(t *testing.T, b *GenericBlock, hashAlgorithm crypto.Hash) (*certificates.UnicityCertificate, map[string]abcrypto.Verifier) {
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
	return uc, map[string]abcrypto.Verifier{"test": verifier}
}

func verifyHashChain(t *testing.T, b *GenericBlock, tx txsystem.GenericTransaction, hashAlgorithm crypto.Hash) {
	unitIdBytes := tx.UnitID().Bytes32()
	leaves, _ := b.blockTreeLeaves(hashAlgorithm)
	chain, _ := treeChain(tx.UnitID(), leaves, hashAlgorithm)
	root := omt.EvalMerklePath(chain, unitIdBytes[:], hashAlgorithm)
	require.Equal(t, "690822883B5310DF3B8DA4232252A76E20E07F2F4FB184CEF85D35DC4AF4DF70", fmt.Sprintf("%X", root),
		"hash chain verification failed for tx=%X", unitIdBytes[:])
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
