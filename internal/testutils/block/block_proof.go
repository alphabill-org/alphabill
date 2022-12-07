package testblock

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

func CreateProof(t *testing.T, tx txsystem.GenericTransaction, signer abcrypto.Signer, unitID []byte) *block.BlockProof {
	b := &block.GenericBlock{}
	if tx != nil {
		b.Transactions = []txsystem.GenericTransaction{tx}
	}
	b.UnicityCertificate = CreateUC(t, b, signer)
	p, err := block.NewPrimaryProof(b, unitID, crypto.SHA256)
	require.NoError(t, err)
	return p
}

func CreatePrimaryProofs(t *testing.T, txs []txsystem.GenericTransaction, signer abcrypto.Signer) (proofs []*block.BlockProof) {
	b := &block.GenericBlock{}
	b.Transactions = txs
	b.UnicityCertificate = CreateUC(t, b, signer)
	for _, tx := range txs {
		p, err := block.NewPrimaryProof(b, util.Uint256ToBytes(tx.UnitID()), crypto.SHA256)
		require.NoError(t, err)
		proofs = append(proofs, p)
	}
	return
}

func CreateUC(t *testing.T, b *block.GenericBlock, signer abcrypto.Signer) *certificates.UnicityCertificate {
	blockHash, _ := b.Hash(crypto.SHA256)
	ir := &certificates.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		BlockHash:    blockHash,
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
	return uc
}

func CertifyBlock(t *testing.T, b *block.Block, txConverter block.TxConverter, signer abcrypto.Signer) *block.Block {
	gblock, err := b.ToGenericBlock(txConverter)
	require.NoError(t, err)
	gblock.UnicityCertificate = CreateUC(t, gblock, signer)
	return gblock.ToProtobuf()
}
