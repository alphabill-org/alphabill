package tokens

import (
	"crypto"
	goerrors "errors"

	"github.com/alphabill-org/alphabill/internal/script"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

const (
	zeroSummaryValue = rma.Uint64SummaryValue(0)

	ErrStrSystemIdentifierIsNil = "system identifier is nil"
	ErrStrUnitIDIsZero          = "unit ID cannot be zero"
	ErrStringInvalidSymbolName  = "symbol name exceeds the allowed maximum length of 64 bytes"
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
	return NewGenericTx(tx)
}

func (t *tokensTxSystem) Execute(tx txsystem.GenericTransaction) error {
	err := txsystem.ValidateGenericTransaction(&txsystem.TxValidationContext{Tx: tx, Bd: nil, SystemIdentifier: t.systemIdentifier, BlockNumber: t.currentBlockNumber})
	if err != nil {
		return err
	}
	switch tx := tx.(type) {
	case *createNonFungibleTokenTypeWrapper:
		if err := t.validateCreateNonFungibleTokenTypeTx(tx); err != nil {
			return err
		}
		h := tx.Hash(t.hashAlgorithm)
		return t.state.AddItem(
			tx.UnitID(),
			script.PredicateAlwaysTrue(),
			newNonFungibleTokenTypeData(tx),
			h,
		)
	default:
		return errors.Errorf("unknown tx type %T", tx)
	}
}

func (t *tokensTxSystem) BeginBlock(blockNr uint64) {
	t.currentBlockNumber = blockNr
}

func (t *tokensTxSystem) EndBlock() (txsystem.State, error) {
	return t.getState(), nil
}

func (t *tokensTxSystem) Revert() {
	t.state.Revert()
}

func (t *tokensTxSystem) Commit() {
	t.state.Commit()
}

func (t *tokensTxSystem) getState() txsystem.State {
	if t.state.GetRootHash() == nil {
		return txsystem.NewStateSummary(make([]byte, t.hashAlgorithm.Size()), zeroSummaryValue.Bytes())
	}
	return txsystem.NewStateSummary(t.state.GetRootHash(), zeroSummaryValue.Bytes())
}

func (t *tokensTxSystem) validateCreateNonFungibleTokenTypeTx(tx *createNonFungibleTokenTypeWrapper) error {
	unitID := tx.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	if len(tx.attributes.Symbol) > 64 {
		return errors.Errorf(ErrStringInvalidSymbolName)
	}
	u, err := t.state.GetUnit(unitID)
	if u != nil {
		return errors.Errorf("unit %v exists", unitID)
	}
	if !goerrors.Is(err, rma.ErrUnitNotFound) {
		return err
	}

	parentTypeID := tx.ParentTypeID()

	// tx proof satisfies the predicate obtained by concatenating all the
	// sub-type creation clauses along the type inheritance chain.
	var predicate []byte
	for {
		if parentTypeID.IsZero() {
			// type has no parent.
			break
		}

		// parent unit must exist
		u, err = t.state.GetUnit(parentTypeID)
		if err != nil {
			return err
		}
		// parent must be a non-fungible token type
		parentData, f := u.Data.(*nonFungibleTokenTypeData)
		if !f {
			return errors.Errorf("unit %v is not a non-fungible token type", parentTypeID)
		}
		predicate = append(parentData.subTypeCreationPredicate, predicate...)
		parentTypeID = parentData.parentTypeId
	}
	if len(predicate) > 0 {
		return script.RunScript(tx.attributes.SubTypeCreationPredicateSignature, predicate, tx.SigBytes())
	}
	return nil
}
