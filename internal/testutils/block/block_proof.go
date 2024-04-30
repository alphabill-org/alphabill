package testblock

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	"github.com/stretchr/testify/require"
)

const (
	DefaultT2Timeout        = 2500
	DefaultRoundNumber      = 1
)

type (
	Options struct {
		sdr *types.SystemDescriptionRecord
	}

	Option func(*Options)
)

func DefaultOptions() *Options {
	return &Options{
		sdr: DefaultSDR(),
	}
}

func DefaultSDR() *types.SystemDescriptionRecord {
	return &types.SystemDescriptionRecord{
		SystemIdentifier: money.DefaultSystemID,
		T2Timeout:        DefaultT2Timeout,
	}
}

func WithSystemIdentifier(systemID types.SystemID) Option {
	return func(g *Options) {
		g.sdr.SystemIdentifier = systemID
	}
}

func CreateProof(t *testing.T, tx *types.TransactionRecord, signer abcrypto.Signer, opts ...Option) *types.TxProof {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	ir := &types.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         test.RandomBytes(32),
		RoundNumber:  DefaultRoundNumber,
		SummaryValue: make([]byte, 32),
	}
	b := CreateBlock(t, []*types.TransactionRecord{tx}, ir, options.sdr, signer)
	p, _, err := types.NewTxProof(b, 0, crypto.SHA256)
	require.NoError(t, err)
	return p
}

func CreateProofs(t *testing.T, txs []*types.TransactionRecord, signer abcrypto.Signer, opts ...Option) []*types.TxProof {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	ir := &types.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         test.RandomBytes(32),
		RoundNumber:  DefaultRoundNumber,
		SummaryValue: make([]byte, 32),
	}
	b := CreateBlock(t, txs, ir, options.sdr, signer)

	var proofs []*types.TxProof
	for i := range txs {
		p, _, err := types.NewTxProof(b, i, crypto.SHA256)
		require.NoError(t, err)
		proofs = append(proofs, p)
	}
	return proofs
}

func CreateBlock(t *testing.T, txs []*types.TransactionRecord, ir *types.InputRecord, sdr *types.SystemDescriptionRecord, signer abcrypto.Signer) *types.Block {
	b := &types.Block{
		Header: &types.Header{
			SystemID:          types.SystemID(1),
			ProposerID:        "test",
			PreviousBlockHash: make([]byte, 32),
		},
		Transactions: txs,
		UnicityCertificate: &types.UnicityCertificate{
			InputRecord: ir,
		},
	}
	// calculate block hash
	blockHash, err := b.Hash(crypto.SHA256)
	require.NoError(t, err)
	ir.BlockHash = blockHash
	b.UnicityCertificate = testcertificates.CreateUnicityCertificate(
		t,
		signer,
		ir,
		sdr,
		1,
		make([]byte, 32),
	)
	return b
}
