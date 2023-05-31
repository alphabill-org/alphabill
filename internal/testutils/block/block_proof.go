package testblock

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

func CreateProof(t *testing.T, tx *types.TransactionRecord, signer abcrypto.Signer) *types.TxProof {
	b := &types.Block{Header: &types.Header{}, UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}}}
	if tx != nil {
		b.Transactions = []*types.TransactionRecord{tx}
	}
	b.UnicityCertificate = CreateUC(t, b, signer)
	p, _, err := types.NewTxProof(b, 0, crypto.SHA256)
	require.NoError(t, err)
	return p
}

func CreateUC(t *testing.T, b *types.Block, signer abcrypto.Signer) *types.UnicityCertificate {
	blockHash, _ := b.Hash(crypto.SHA256)
	ir := &types.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		BlockHash:    blockHash,
		RoundNumber:  b.UnicityCertificate.InputRecord.RoundNumber,
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
