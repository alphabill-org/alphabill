package testblock

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func CreateProof(t *testing.T, tx txsystem.GenericTransaction, signer abcrypto.Signer, unitID *uint256.Int) *block.BlockProof {
	b := &block.GenericBlock{}
	if tx != nil {
		b.Transactions = []txsystem.GenericTransaction{tx}
	}
	b.UnicityCertificate = CreateUC(t, b, signer)
	p, err := block.NewPrimaryProof(b, unitID, crypto.SHA256)
	require.NoError(t, err)
	return p
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

func CertifyBlock(t *testing.T, b *block.Block, txConverter block.TxConverter) (*block.Block, map[string]abcrypto.Verifier) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	verifiers := map[string]abcrypto.Verifier{"test": verifier}
	genericBlock, _ := b.ToGenericBlock(txConverter)
	genericBlock.UnicityCertificate = CreateUC(t, genericBlock, signer)
	return genericBlock.ToProtobuf(), verifiers
}
