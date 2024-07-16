package permissioned

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	feeModule "github.com/alphabill-org/alphabill/txsystem/fc"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

var _ txsystem.FeeCreditModule = (*FeeCreditModule)(nil)

const GasUnitsPerTema = 1000

type (
	// FeeCreditModule is a transaction system module for handling fees in "permissioned" mode.
	// In permissioned mode all FCRs must be created by a designated "admin key" with a CreateFCR transaction.
	// Furthermore, all transactions are free e.g. cost 0, however, the transaction fee proof (or owner proof)
	// must satisfy the FCR i.e. all users must ask the owner of the admin key for permission to send transactions.
	FeeCreditModule struct {
		systemIdentifier        types.SystemID
		state                   *state.State
		hashAlgorithm           crypto.Hash
		trustBase               types.RootTrustBase
		execPredicate           predicates.PredicateRunner
		feeCreditRecordUnitType []byte
		feeBalanceValidator     *feeModule.FeeBalanceValidator
	}
)

func NewFeeCreditModule(opts ...Option) (*FeeCreditModule, error) {
	m := &FeeCreditModule{
		hashAlgorithm: crypto.SHA256,
	}
	for _, o := range opts {
		o(m)
	}
	if m.execPredicate == nil {
		predEng, err := predicates.Dispatcher(templates.New())
		if err != nil {
			return nil, fmt.Errorf("creating predicate executor: %w", err)
		}
		m.execPredicate = predicates.NewPredicateRunner(predEng.Execute)
	}
	if m.feeBalanceValidator == nil {
		m.feeBalanceValidator = feeModule.NewFeeBalanceValidator(m.state, m.execPredicate, m.feeCreditRecordUnitType)
	}
	if err := m.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid fee credit module configuration: %w", err)
	}
	return m, nil
}

func (f *FeeCreditModule) CalculateCost(gasUsed uint64) uint64 {
	return 0 // all transactions are "free" in permissioned mode
}

func (f *FeeCreditModule) BuyGas(maxTxCost uint64) uint64 {
	// FCRs have balance of 1 alpha that is never decreased,
	// so transactions cannot spend gas worth more than 1 alpha
	return 1e8 * GasUnitsPerTema
}

func (f *FeeCreditModule) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		fc.PayloadTypeAddFeeCredit:   txtypes.NewTxHandler[CreateFCRAttr](f.validateCreateFC, f.executeCreateFC),
		fc.PayloadTypeCloseFeeCredit: txtypes.NewTxHandler[DeleteFCRAttr](f.validateDeleteFC, f.executeDeleteFC),
	}
}

func (f *FeeCreditModule) IsCredible(exeCtx txtypes.ExecutionContext, tx *types.TransactionOrder) error {
	return f.feeBalanceValidator.IsCredible(exeCtx, tx)
}

func (f *FeeCreditModule) IsValid() error {
	if f.systemIdentifier == 0 {
		return ErrSystemIdentifierMissing
	}
	if f.state == nil {
		return ErrStateIsNil
	}
	if f.trustBase == nil {
		return ErrTrustBaseIsNil
	}
	return nil
}

type CreateFCRAttr struct {
	_ struct{} `cbor:",toarray"`
	// TODO impl
}

type DeleteFCRAttr struct {
	_ struct{} `cbor:",toarray"`
	// TODO impl
}

func (f *FeeCreditModule) validateCreateFC(tx *types.TransactionOrder, attr *CreateFCRAttr, exeCtx txtypes.ExecutionContext) error {
	// TODO impl
	return nil
}

func (f *FeeCreditModule) executeCreateFC(tx *types.TransactionOrder, attr *CreateFCRAttr, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// TODO impl
	return nil, nil
}

func (f *FeeCreditModule) validateDeleteFC(tx *types.TransactionOrder, attr *DeleteFCRAttr, exeCtx txtypes.ExecutionContext) error {
	// TODO impl
	return nil
}

func (f *FeeCreditModule) executeDeleteFC(tx *types.TransactionOrder, attr *DeleteFCRAttr, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// TODO impl
	return nil, nil
}
