package block

import (
	"crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/omt"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	OnlyPrimaryTx struct {
		txsystem.GenericTransaction
	}
	OnlySecondaryTx struct {
		txsystem.GenericTransaction
	}
	MultiUnitTargetTxType struct {
		txsystem.GenericTransaction
		secondaryTargetUnitID *uint256.Int
	}
)

func (g *OnlyPrimaryTx) IsPrimary() bool {
	return true
}

func (g *OnlySecondaryTx) IsPrimary() bool {
	return false
}

func (d *MultiUnitTargetTxType) TargetUnits(_ crypto.Hash) []*uint256.Int {
	return []*uint256.Int{d.UnitID(), d.secondaryTargetUnitID}
}

func TestProofTypePrim(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	b := &GenericBlock{}
	var maxTx uint64 = 10
	for num := uint64(1); num <= maxTx; num++ {
		b.Transactions = append(b.Transactions, createPrimaryTx(num))
	}
	uc, verifier := createUC(t, b, hashAlgorithm)
	b.UnicityCertificate = uc

	// verify primary proof for each transaction
	for i, tx := range b.Transactions {
		verifyHashChain(t, b, tx, hashAlgorithm)

		unitIDBytes := util.Uint256ToBytes(tx.UnitID())
		p, err := NewPrimaryProof(b, unitIDBytes, hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, ProofType_PRIM, p.ProofType)
		require.Nil(t, p.Verify(unitIDBytes, tx, verifier, hashAlgorithm),
			"proof verification failed for tx_idx=%d", i)

		// test proof does not verify for wrong transaction
		tx = createPrimaryTx(maxTx + 1)
		err = p.Verify(unitIDBytes, tx, verifier, hashAlgorithm)
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
		unitIDBytes := util.Uint256ToBytes(tx.UnitID())
		p, err := NewSecondaryProof(b, unitIDBytes, i, hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, ProofType_SEC, p.ProofType)
		require.Nil(t, p.Verify(unitIDBytes, tx, verifier, hashAlgorithm),
			"proof verification failed for tx_idx=%d", i)

		// test proof does not verify invalid tx
		nonExistentTxInBlock := createSecondaryTx(2)
		require.ErrorIs(t, p.Verify(util.Uint256ToBytes(nonExistentTxInBlock.UnitID()), nonExistentTxInBlock, verifier, hashAlgorithm), ErrProofVerificationFailed,
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
		unitIDBytes := util.Uint256ToBytes(tx.UnitID())
		p, err := NewPrimaryProof(b, unitIDBytes, hashAlgorithm)
		require.NoError(t, err)
		require.Equal(t, ProofType_ONLYSEC, p.ProofType)
		require.Nil(t, p.Verify(unitIDBytes, tx, verifier, hashAlgorithm),
			"proof verification failed for tx_idx=%d", i)

		// test proof does not verify for wrong transaction
		tx = createPrimaryTx(2)
		err = p.Verify(util.Uint256ToBytes(tx.UnitID()), tx, verifier, hashAlgorithm)
		require.ErrorIs(t, err, ErrProofVerificationFailed)

		//tx = createPrimaryTx(1) // TODO This case fails, but AhtoB says it's by design
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
	unitIDBytes := util.Uint256ToBytes(tx.UnitID())
	p, err := NewPrimaryProof(b, unitIDBytes, hashAlgorithm)
	require.NoError(t, err)
	require.Equal(t, ProofType_NOTRANS, p.ProofType)
	require.Nil(t, p.Verify(unitIDBytes, tx, verifier, hashAlgorithm))
}

func TestProofTypeEmptyBlock(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	b := &GenericBlock{}
	uc, verifier := createUC(t, b, hashAlgorithm)
	b.UnicityCertificate = uc

	unitIDBytes := util.Uint256ToBytes(uint256.NewInt(1))
	p, err := NewPrimaryProof(b, unitIDBytes, hashAlgorithm)
	require.NoError(t, err)
	require.Equal(t, ProofType_EMPTYBLOCK, p.ProofType)
	tx := createPrimaryTx(1)
	require.Nil(t, p.Verify(unitIDBytes, tx, verifier, hashAlgorithm))
}

func TestProofsForSecondaryTargetUnits(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	nonExistentTx := createMultiTargetTx(1337, 1338)
	b := &GenericBlock{
		Transactions: []txsystem.GenericTransaction{
			createPrimaryTx(1),
			createMultiTargetTx(2, 3),
			createMultiTargetTx(4, 5),
			createPrimaryTx(6),
		},
	}
	uc, verifier := createUC(t, b, hashAlgorithm)
	b.UnicityCertificate = uc

	for i, gtx := range b.Transactions {
		units := gtx.TargetUnits(hashAlgorithm)
		for _, unitID := range units {
			msg := fmt.Sprintf("tx_idx=%d unitID=%X", i, unitID)
			unitIDBytes := util.Uint256ToBytes(unitID)
			p, err := NewPrimaryProof(b, unitIDBytes, hashAlgorithm)
			require.NoError(t, err, msg)
			require.Equal(t, ProofType_PRIM, p.ProofType, msg)
			require.Nil(t, p.Verify(unitIDBytes, gtx, verifier, hashAlgorithm), msg)

			// test valid unitID with non-block tx returns errors
			err = p.Verify(unitIDBytes, nonExistentTx, verifier, hashAlgorithm)
			require.ErrorIs(t, err, ErrProofVerificationFailed, msg)
		}
	}
}

func createPrimaryTx(unitID uint64) *OnlyPrimaryTx {
	transaction := newTransaction(test.NewUnitID(unitID), make([]byte, 32), 555)
	tx, _ := txsystem.NewDefaultGenericTransaction(transaction)
	return &OnlyPrimaryTx{tx}
}

func createSecondaryTx(unitID uint64) *OnlySecondaryTx {
	tx := createPrimaryTx(unitID)
	return &OnlySecondaryTx{tx}
}

func createMultiTargetTx(unitID uint64, secUnitID uint64) *MultiUnitTargetTxType {
	transaction := newTransaction(test.NewUnitID(unitID), make([]byte, 32), 555)
	tx, _ := txsystem.NewDefaultGenericTransaction(transaction)
	return &MultiUnitTargetTxType{tx, uint256.NewInt(secUnitID)}
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
	unitIDBytes := util.Uint256ToBytes(tx.UnitID())
	leaves, _ := b.blockTreeLeaves(hashAlgorithm)
	chain, _ := treeChain(unitIDBytes, leaves, hashAlgorithm)
	root := omt.EvalMerklePath(chain, unitIDBytes[:], hashAlgorithm)
	require.Equal(t, "690822883B5310DF3B8DA4232252A76E20E07F2F4FB184CEF85D35DC4AF4DF70", fmt.Sprintf("%X", root),
		"hash chain verification failed for tx=%X", unitIDBytes[:])
}

func newTransaction(id, ownerProof []byte, timeout uint64) *txsystem.Transaction {
	tx := &txsystem.Transaction{
		SystemId:              []byte{0, 0, 0, 0},
		UnitId:                id,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
	return tx
}
