package permissioned

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	feeModule "github.com/alphabill-org/alphabill/txsystem/fc"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

var _ txtypes.FeeCreditModule = (*FeeCreditModule)(nil)

var (
	ErrMissingSystemIdentifier        = errors.New("system identifier is missing")
	ErrStateIsNil                     = errors.New("state is nil")
	ErrMissingFeeCreditRecordUnitType = errors.New("fee credit record unit type is missing")
	ErrMissingAdminOwnerCondition     = errors.New("admin owner condition is missing")
)

type (
	// FeeCreditModule is a transaction system module for handling fees in "permissioned" mode.
	// In permissioned mode all FCRs must be created by a designated "admin key" with a CreateFCR transaction.
	// Furthermore, all transactions are free e.g. cost 0, however, the transaction fee proof (or owner proof)
	// must satisfy the FCR i.e. all users must ask the owner of the admin key for permission to send transactions.
	FeeCreditModule struct {
		systemIdentifier        types.SystemID
		state                   *state.State
		hashAlgorithm           crypto.Hash
		execPredicate           predicates.PredicateRunner
		feeCreditRecordUnitType []byte
		feeBalanceValidator     *feeModule.FeeBalanceValidator
		adminOwnerCondition     types.PredicateBytes
	}
)

func NewFeeCreditModule(systemID types.SystemID, state *state.State, feeCreditRecordUnitType []byte, adminOwnerCondition []byte, opts ...Option) (*FeeCreditModule, error) {
	if systemID == 0 {
		return nil, ErrMissingSystemIdentifier
	}
	if state == nil {
		return nil, ErrStateIsNil
	}
	if len(feeCreditRecordUnitType) == 0 {
		return nil, ErrMissingFeeCreditRecordUnitType
	}
	if len(adminOwnerCondition) == 0 {
		return nil, ErrMissingAdminOwnerCondition
	}
	m := &FeeCreditModule{
		systemIdentifier:        systemID,
		state:                   state,
		feeCreditRecordUnitType: feeCreditRecordUnitType,
		adminOwnerCondition:     adminOwnerCondition,
		hashAlgorithm:           crypto.SHA256,
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
	return m, nil
}

func (f *FeeCreditModule) CalculateCost(gasUsed uint64) uint64 {
	return 0 // all transactions are "free" in permissioned mode
}

func (f *FeeCreditModule) BuyGas(maxTxCost uint64) uint64 {
	// FCRs have balance of 1 alpha that is never decreased,
	// so transactions cannot spend gas worth more than 1 alpha
	return 1e8 * feeModule.GasUnitsPerTema
}

func (f *FeeCreditModule) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		permissioned.PayloadTypeCreateFCR: txtypes.NewTxHandler[permissioned.CreateFeeCreditAttributes](f.validateCreateFCR, f.executeCreateFCR),
		permissioned.PayloadTypeDeleteFCR: txtypes.NewTxHandler[permissioned.DeleteFeeCreditAttributes](f.validateDeleteFCR, f.executeDeleteFCR),
	}
}

func (f *FeeCreditModule) IsFeeCreditTx(tx *types.TransactionOrder) bool {
	return permissioned.IsFeeCreditTx(tx)
}

func (f *FeeCreditModule) IsCredible(exeCtx txtypes.ExecutionContext, tx *types.TransactionOrder) error {
	return f.feeBalanceValidator.IsCredible(exeCtx, tx)
}
