package tokens

import (
	"bytes"
	"crypto"
	goerrors "errors"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const (
	zeroSummaryValue = rma.Uint64SummaryValue(0)
	uriMaxSize       = 4 * 1024
	dataMaxSize      = 64 * 1024

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
		if err = t.validateCreateNonFungibleTokenTypeTx(tx); err != nil {
			return err
		}
		h := tx.Hash(t.hashAlgorithm)
		return t.state.AddItem(
			tx.UnitID(),
			script.PredicateAlwaysTrue(),
			newNonFungibleTokenTypeData(tx),
			h,
		)
	case *mintNonFungibleTokenWrapper:
		if err = t.validateMintNonFungibleTokenWrapper(tx); err != nil {
			return err
		}
		h := tx.Hash(t.hashAlgorithm)
		return t.state.AddItem(
			tx.UnitID(),
			tx.attributes.Bearer,
			newMintNonFungibleTokenData(tx, t.hashAlgorithm),
			h,
		)
	case *transferNonFungibleTokenWrapper:
		if err = t.validateTransferNonFungibleTokenWrapper(tx); err != nil {
			return err
		}
		h := tx.Hash(t.hashAlgorithm)
		if err = t.state.SetOwner(tx.UnitID(), tx.attributes.NewBearer, h); err != nil {
			return err
		}
		return t.state.UpdateData(tx.UnitID(), func(data rma.UnitData) (newData rma.UnitData) {
			d, ok := data.(*nonFungibleTokenData)
			if !ok {
				return data
			}
			d.t = t.currentBlockNumber
			d.backlink = tx.Hash(t.hashAlgorithm)
			return data
		}, h)
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
	// signature satisfies the predicate obtained by concatenating all the
	// sub-type creation clauses along the type inheritance chain.
	predicate, err := t.getChainedPredicate(
		tx.ParentTypeID(),
		func(d *nonFungibleTokenTypeData) []byte {
			return d.subTypeCreationPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	if len(predicate) > 0 {
		return script.RunScript(tx.attributes.SubTypeCreationPredicateSignature, predicate, tx.SigBytes())
	}
	return nil
}

func (t *tokensTxSystem) validateMintNonFungibleTokenWrapper(tx *mintNonFungibleTokenWrapper) error {
	unitID := tx.wrapper.UnitID()
	unitID.Uint64()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	uri := tx.attributes.Uri
	if len(uri) > uriMaxSize {
		return errors.Errorf("URI exceeds the maximum allowed size of %v KB", uriMaxSize)
	}
	if !util.IsValidURI(uri) {
		return errors.Errorf("URI %s is invalid", uri)
	}
	if len(tx.attributes.Data) > dataMaxSize {
		return errors.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	u, err := t.state.GetUnit(unitID)
	if u != nil {
		return errors.Errorf("unit %v exists", unitID)
	}
	if !goerrors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	nftTypeID := tx.NFTTypeID()
	if nftTypeID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}

	// the transaction request satisfies the predicate obtained by concatenating all the token creation clauses along
	// the type inheritance chain.
	predicate, err := t.getChainedPredicate(
		nftTypeID,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.tokenCreationPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	if len(predicate) > 0 {
		return script.RunScript(tx.attributes.TokenCreationPredicateSignature, predicate, tx.SigBytes())
	}
	return nil
}

func (t *tokensTxSystem) validateTransferNonFungibleTokenWrapper(tx *transferNonFungibleTokenWrapper) error {
	unitID := tx.UnitID()
	u, err := t.state.GetUnit(unitID)
	if err != nil {
		return err
	}
	data, ok := u.Data.(*nonFungibleTokenData)
	if !ok {
		return errors.Errorf("unit %v is not a non-fungible token type", unitID)
	}
	if !bytes.Equal(data.backlink, tx.attributes.Backlink) {
		return errors.New("invalid backlink")
	}
	// signature given in the transaction request satisfies the predicate obtained by concatenating all the token
	// invariant clauses along the type inheritance chain.
	predicate, err := t.getChainedPredicate(
		data.nftType,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return script.RunScript(tx.attributes.InvariantPredicateSignature, predicate, tx.SigBytes())
}

func (t *tokensTxSystem) getChainedPredicate(unitID *uint256.Int, predicateFn func(d *nonFungibleTokenTypeData) []byte, parentIDFn func(d *nonFungibleTokenTypeData) *uint256.Int) ([]byte, error) {
	var predicate []byte
	var parentID = unitID
	for {
		if parentID.IsZero() {
			// type has no parent.
			break
		}
		// parent unit must exist
		u, err := t.state.GetUnit(parentID)
		if err != nil {
			return nil, err
		}

		// parent must be a non-fungible token type
		parentData, f := u.Data.(*nonFungibleTokenTypeData)
		if !f {
			return nil, errors.Errorf("unit %v is not a non-fungible token type", parentID)
		}
		predicate = append(predicateFn(parentData), predicate...)
		parentID = parentData.parentTypeId
	}
	return predicate, nil
}
