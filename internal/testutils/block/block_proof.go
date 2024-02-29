package testblock

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

const (
	DefaultSystemIdentifier = 0x00000001
	DefaultT2Timeout        = 2500
	DefaultRoundNumber      = 1
)

type (
	Options struct {
		sdr *genesis.SystemDescriptionRecord
	}

	Option func(*Options)
)

func DefaultOptions() *Options {
	return &Options{
		sdr: DefaultSDR(),
	}
}

func DefaultSDR() *genesis.SystemDescriptionRecord {
	return &genesis.SystemDescriptionRecord{
		SystemIdentifier: DefaultSystemIdentifier,
		T2Timeout:        DefaultT2Timeout,
	}
}

func WithSystemIdentifier(systemID types.SystemID) Option {
	return func(g *Options) {
		g.sdr.SystemIdentifier = systemID
	}
}

func CalculateBlockHash(t *testing.T, block *types.Block, ir *types.InputRecord) []byte {
	block.UnicityCertificate = &types.UnicityCertificate{
		InputRecord: ir,
	}
	blockHash, err := block.Hash(crypto.SHA256)
	require.NoError(t, err)
	return blockHash
}

func CreateProof(t *testing.T, tx *types.TransactionRecord, signer abcrypto.Signer, opts ...Option) *types.TxProof {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	b := &types.Block{
		Header:       &types.Header{},
		Transactions: []*types.TransactionRecord{tx},
	}
	b.UnicityCertificate = CreateUC(t, b, DefaultRoundNumber, options.sdr, signer)

	p, _, err := types.NewTxProof(b, 0, crypto.SHA256)
	require.NoError(t, err)
	return p
}

func CreateProofs(t *testing.T, txs []*types.TransactionRecord, signer abcrypto.Signer, opts ...Option) []*types.TxProof {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	b := &types.Block{
		Header:       &types.Header{},
		Transactions: txs,
	}
	b.UnicityCertificate = CreateUC(t, b, DefaultRoundNumber, options.sdr, signer)

	var proofs []*types.TxProof
	for i := range txs {
		p, _, err := types.NewTxProof(b, i, crypto.SHA256)
		require.NoError(t, err)
		proofs = append(proofs, p)
	}
	return proofs
}

func CreateUC(t *testing.T, b *types.Block, roundNumber uint64, sdr *genesis.SystemDescriptionRecord, signer abcrypto.Signer) *types.UnicityCertificate {
	ir := &types.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		RoundNumber:  roundNumber,
		SummaryValue: make([]byte, 32),
	}
	// simulate state hash change if there are transactions in the block
	if len(b.Transactions) > 0 {
		ir.Hash[0] += 1
	}
	blockHash := CalculateBlockHash(t, b, ir)
	ir.BlockHash = blockHash
	return testcertificates.CreateUnicityCertificate(
		t,
		signer,
		ir,
		sdr,
		1,
		make([]byte, 32),
	)
}
