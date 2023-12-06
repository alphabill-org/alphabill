package testblock

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/testutils/certificates"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

var (
	DefaultSystemIdentifier = []byte{0, 0, 0, 0}
	DefaultT2Timeout        = uint32(2500)
	DefaultRoundNumber      = uint64(1)
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

func WithSystemIdentifier(systemID []byte) Option {
	return func(g *Options) {
		g.sdr.SystemIdentifier = systemID
	}
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
	blockHash, err := b.Hash(crypto.SHA256)
	require.NoError(t, err)
	ir := &types.InputRecord{
		PreviousHash: make([]byte, 32),
		Hash:         make([]byte, 32),
		BlockHash:    blockHash,
		RoundNumber:  roundNumber,
		SummaryValue: make([]byte, 32),
	}
	return testcertificates.CreateUnicityCertificate(
		t,
		signer,
		ir,
		sdr,
		1,
		make([]byte, 32),
	)
}
