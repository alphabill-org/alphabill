package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

const (
	zeroSummaryValue = rma.Uint64SummaryValue(0)

	ErrStrSystemIdentifierIsNil = "system identifier is nil"
)

type (
	tokensTxSystem struct {
		systemIdentifier   []byte
		state              *rma.Tree
		hashAlgorithm      crypto.Hash
		currentBlockNumber uint64
	}
)

func New(opts ...Option) (*tokensTxSystem, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	if options.systemIdentifier == nil {
		return nil, errors.New(ErrStrSystemIdentifierIsNil)
	}
	state, err := rma.New(&rma.Config{
		HashAlgorithm: options.hashAlgorithm,
	})
	if err != nil {
		return nil, err
	}

	txs := &tokensTxSystem{
		systemIdentifier: options.systemIdentifier,
		hashAlgorithm:    options.hashAlgorithm,
		state:            state,
	}
	logger.Info("TokensTransactionSystem initialized: systemIdentifier=%X, hashAlgorithm=%v", options.systemIdentifier, options.hashAlgorithm)
	return txs, nil
}

func (t *tokensTxSystem) State() (txsystem.State, error) {
	if t.state.ContainsUncommittedChanges() {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	return t.getState(), nil
}

func (t *tokensTxSystem) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	// TODO tasks AB-340 - AB-347
	panic("not implemented")
}

func (t *tokensTxSystem) Execute(tx txsystem.GenericTransaction) error {
	// TODO tasks AB-340 - AB-347
	panic("not implemented")
}

func (t *tokensTxSystem) BeginBlock(blockNr uint64) {
	t.currentBlockNumber = blockNr
	// TODO tasks AB-340 - AB-347
}

func (t *tokensTxSystem) EndBlock() (txsystem.State, error) {
	// TODO tasks AB-340 - AB-347
	return t.getState(), nil
}

func (t *tokensTxSystem) Revert() {
	// TODO tasks AB-340 - AB-347
	t.state.Revert()
}

func (t *tokensTxSystem) Commit() {
	// TODO tasks AB-340 - AB-347
	t.state.Commit()
}

func (t *tokensTxSystem) getState() txsystem.State {
	if t.state.GetRootHash() == nil {
		return txsystem.NewStateSummary(make([]byte, t.hashAlgorithm.Size()), zeroSummaryValue.Bytes())
	}
	return txsystem.NewStateSummary(t.state.GetRootHash(), zeroSummaryValue.Bytes())
}
