package testblock

import (
	"crypto"
	"testing"
	"time"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	"github.com/stretchr/testify/require"
)

const (
	DefaultRoundNumber = 1
)

type (
	Options struct {
		sdr *types.PartitionDescriptionRecord
	}

	Option func(*Options)
)

func DefaultOptions() *Options {
	return &Options{
		sdr: DefaultSDR(),
	}
}

func DefaultSDR() *types.PartitionDescriptionRecord {
	return &types.PartitionDescriptionRecord{
		NetworkIdentifier: 5,
		SystemIdentifier:  money.DefaultSystemID,
		T2Timeout:         2500 * time.Millisecond,
	}
}

func WithSystemIdentifier(systemID types.SystemID) Option {
	return func(g *Options) {
		g.sdr.SystemIdentifier = systemID
	}
}

func CreateTxRecordProof(t *testing.T, txRecord *types.TransactionRecord, signer abcrypto.Signer, opts ...Option) *types.TxRecordProof {
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
	b := CreateBlock(t, []*types.TransactionRecord{txRecord}, ir, options.sdr, signer)
	p, err := types.NewTxRecordProof(b, 0, crypto.SHA256)
	require.NoError(t, err)
	return p
}

func CreateBlock(t *testing.T, txs []*types.TransactionRecord, ir *types.InputRecord, sdr *types.PartitionDescriptionRecord, signer abcrypto.Signer) *types.Block {
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
