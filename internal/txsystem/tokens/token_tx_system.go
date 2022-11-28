package tokens

import (
	"crypto"
	"reflect"

	"github.com/alphabill-org/alphabill/internal/block"
	"google.golang.org/protobuf/proto"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

const (
	zeroSummaryValue = rma.Uint64SummaryValue(0)
	uriMaxSize       = 4 * 1024
	dataMaxSize      = 64 * 1024
	maxSymbolLength  = 64
	maxDecimalPlaces = 8

	ErrStrSystemIdentifierIsNil = "system identifier is nil"
	ErrStrStateIsNil            = "state is nil"
	ErrStrUnitIDIsZero          = "unit ID cannot be zero"
	ErrStrInvalidSymbolName     = "symbol name exceeds the allowed maximum length of 64 bytes"
)

type (
	TokenState interface {
		revertibleState
		AtomicUpdate(actions ...rma.Action) error
		GetUnit(id *uint256.Int) (*rma.Unit, error)
	}

	revertibleState interface {
		ContainsUncommittedChanges() bool
		GetRootHash() []byte
		Commit()
		Revert()
	}

	tokensTxSystem struct {
		systemIdentifier   []byte
		state              TokenState
		hashAlgorithm      crypto.Hash
		currentBlockNumber uint64
		executors          map[reflect.Type]txExecutor
	}

	txExecutor interface {
		Execute(tx txsystem.GenericTransaction, currentBlockNr uint64) error
	}
)

// token tx type interfaces
type (
	CreateNonFungibleTokenType interface {
		txsystem.GenericTransaction
		ParentTypeID() []byte
		Symbol() string
		SubTypeCreationPredicate() []byte
		TokenCreationPredicate() []byte
		InvariantPredicate() []byte
		DataUpdatePredicate() []byte
		SubTypeCreationPredicateSignatures() [][]byte
	}

	MintNonFungibleToken interface {
		txsystem.GenericTransaction
		NFTTypeID() []byte
		NFTTypeIDInt() *uint256.Int
		Bearer() []byte
		URI() string
		Data() []byte
		DataUpdatePredicate() []byte
		TokenCreationPredicateSignatures() [][]byte
	}

	TransferNonFungibleToken interface {
		txsystem.GenericTransaction
		NFTTypeID() []byte
		NewBearer() []byte
		Nonce() []byte
		Backlink() []byte
		InvariantPredicateSignature() []byte
	}

	UpdateNonFungibleToken interface {
		txsystem.GenericTransaction
		Data() []byte
		Backlink() []byte
		DataUpdateSignature() []byte
	}

	CreateFungibleTokenType interface {
		txsystem.GenericTransaction
		ParentTypeID() []byte
		Symbol() string
		DecimalPlaces() uint32
		SubTypeCreationPredicate() []byte
		TokenCreationPredicate() []byte
		InvariantPredicate() []byte
		SubTypeCreationPredicateSignatures() [][]byte
	}

	MintFungibleToken interface {
		txsystem.GenericTransaction
		TypeIDInt() *uint256.Int
		TypeID() []byte
		Value() uint64
		Bearer() []byte
		TokenCreationPredicateSignatures() [][]byte
	}

	TransferFungibleToken interface {
		txsystem.GenericTransaction
		TypeID() []byte
		NewBearer() []byte
		Value() uint64
		Nonce() []byte
		Backlink() []byte
		InvariantPredicateSignature() []byte
	}

	SplitFungibleToken interface {
		txsystem.GenericTransaction
		TypeID() []byte
		HashForIDCalculation(hashFunc crypto.Hash) []byte
		NewBearer() []byte
		TargetValue() uint64
		RemainingValue() uint64
		Nonce() []byte
		Backlink() []byte
		InvariantPredicateSignature() []byte
	}

	BurnFungibleToken interface {
		txsystem.GenericTransaction
		TypeID() []byte
		Value() uint64
		Nonce() []byte
		Backlink() []byte
		InvariantPredicateSignature() []byte
	}

	JoinFungibleToken interface {
		txsystem.GenericTransaction
		BurnTransactions() []BurnFungibleToken
		BlockProofs() []*block.BlockProof
		Backlink() []byte
		InvariantPredicateSignature() []byte
	}

	MintAttr interface {
		proto.Message
		SetBearer([]byte)
		SetTokenCreationPredicateSignatures([][]byte)
	}

	AttrWithSubTypeCreationInputs interface {
		proto.Message
		SetSubTypeCreationPredicateSignatures([][]byte)
	}
)

func (x *CreateFungibleTokenTypeAttributes) SetSubTypeCreationPredicateSignatures(sigs [][]byte) {
	x.SubTypeCreationPredicateSignatures = sigs
}

func (x *CreateNonFungibleTokenTypeAttributes) SetSubTypeCreationPredicateSignatures(sigs [][]byte) {
	x.SubTypeCreationPredicateSignatures = sigs
}

func (x *MintFungibleTokenAttributes) SetBearer(b []byte) {
	x.Bearer = b
}

func (x *MintFungibleTokenAttributes) SetTokenCreationPredicateSignatures(sigs [][]byte) {
	x.TokenCreationPredicateSignatures = sigs
}

func (x *MintNonFungibleTokenAttributes) SetBearer(b []byte) {
	x.Bearer = b
}

func (x *MintNonFungibleTokenAttributes) SetTokenCreationPredicateSignatures(sigs [][]byte) {
	x.TokenCreationPredicateSignatures = sigs
}

func New(opts ...Option) (*tokensTxSystem, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(options)
	}
	if options.systemIdentifier == nil {
		return nil, errors.New(ErrStrSystemIdentifierIsNil)
	}

	if options.state == nil {
		return nil, errors.New(ErrStrStateIsNil)
	}

	txs := &tokensTxSystem{
		systemIdentifier: options.systemIdentifier,
		hashAlgorithm:    options.hashAlgorithm,
		state:            options.state,
		executors:        initExecutors(options.state, options),
	}
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
	txType := reflect.TypeOf(tx)
	executor := t.executors[txType]
	if executor == nil {
		return errors.Errorf("unknown tx type %T", tx)
	}
	return executor.Execute(tx, t.currentBlockNumber)
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

func initExecutors(state TokenState, options *Options) map[reflect.Type]txExecutor {
	executors := make(map[reflect.Type]txExecutor)
	// non-fungible token tx executors
	commonNFTTxExecutor := &baseTxExecutor[*nonFungibleTokenTypeData]{
		state:         state,
		hashAlgorithm: options.hashAlgorithm,
	}
	executors[reflect.TypeOf(&createNonFungibleTokenTypeWrapper{})] = &createNonFungibleTokenTypeTxExecutor{commonNFTTxExecutor}
	executors[reflect.TypeOf(&mintNonFungibleTokenWrapper{})] = &mintNonFungibleTokenTxExecutor{commonNFTTxExecutor}
	executors[reflect.TypeOf(&transferNonFungibleTokenWrapper{})] = &transferNonFungibleTokenTxExecutor{commonNFTTxExecutor}
	executors[reflect.TypeOf(&updateNonFungibleTokenWrapper{})] = &updateNonFungibleTokenTxExecutor{commonNFTTxExecutor}

	// fungible token tx executors
	commonFungibleTokenTxExecutor := &baseTxExecutor[*fungibleTokenTypeData]{
		state:         state,
		hashAlgorithm: options.hashAlgorithm,
	}
	executors[reflect.TypeOf(&createFungibleTokenTypeWrapper{})] = &createFungibleTokenTypeTxExecutor{commonFungibleTokenTxExecutor}
	executors[reflect.TypeOf(&mintFungibleTokenWrapper{})] = &mintFungibleTokenTxExecutor{commonFungibleTokenTxExecutor}
	executors[reflect.TypeOf(&transferFungibleTokenWrapper{})] = &transferFungibleTokenTxExecutor{commonFungibleTokenTxExecutor}
	executors[reflect.TypeOf(&splitFungibleTokenWrapper{})] = &splitFungibleTokenTxExecutor{commonFungibleTokenTxExecutor}
	executors[reflect.TypeOf(&burnFungibleTokenWrapper{})] = &burnFungibleTokenTxExecutor{commonFungibleTokenTxExecutor}
	executors[reflect.TypeOf(&joinFungibleTokenWrapper{})] = &joinFungibleTokenTxExecutor{baseTxExecutor: commonFungibleTokenTxExecutor, trustBase: options.trustBase}

	return executors
}
