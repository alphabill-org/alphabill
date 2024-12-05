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
	ErrMissingNetworkID       = errors.New("network identifier is missing")
	ErrMissingPartitionID             = errors.New("partition identifier is missing")
	ErrStateIsNil                     = errors.New("state is nil")
	ErrMissingFeeCreditRecordUnitType = errors.New("fee credit record unit type is missing")
	ErrMissingAdminOwnerPredicate     = errors.New("admin owner predicate is missing")
)

/*
FeeCreditModule is a transaction system module for handling fees in "permissioned" mode.

In permissioned mode there are two special transactions: SetFC and DeleteFC;
these transactions can only be sent by the operator of this partition i.e. owner of the admin key.
The SetFC transaction can be used to create new fee credit records and update existing ones.
The DeleteFC transaction can be used to close existing fee credit records.
All other ordinary transactions must still satisfy the fee credit records
i.e. users must ask the owner of the partition for permission to send transactions.

In addition, the module can be configured in two modes: normal and feeless.
In normal mode the non-fee transaction costs are calculated normally.
In feeless mode the non-fee transactions are "free" i.e. no actual fees are charged.
*/
type FeeCreditModule struct {
	networkID               types.NetworkID
	partitionID             types.PartitionID
	state                   *state.State
	hashAlgorithm           crypto.Hash
	execPredicate           predicates.PredicateRunner
	feeCreditRecordUnitType []byte
	feeBalanceValidator     *feeModule.FeeBalanceValidator
	adminOwnerPredicate     types.PredicateBytes
	feelessMode             bool
}

func NewFeeCreditModule(networkID types.NetworkID, partitionID types.PartitionID, state *state.State, feeCreditRecordUnitType []byte, adminOwnerPredicate []byte, opts ...Option) (*FeeCreditModule, error) {
	if networkID == 0 {
		return nil, ErrMissingPartitionID
	}
	if partitionID == 0 {
		return nil, ErrMissingPartitionID
	}
	if state == nil {
		return nil, ErrStateIsNil
	}
	if len(feeCreditRecordUnitType) == 0 {
		return nil, ErrMissingFeeCreditRecordUnitType
	}
	if len(adminOwnerPredicate) == 0 {
		return nil, ErrMissingAdminOwnerPredicate
	}
	m := &FeeCreditModule{
		partitionID:     partitionID,
		state:                   state,
		feeCreditRecordUnitType: feeCreditRecordUnitType,
		adminOwnerPredicate:     adminOwnerPredicate,
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

// CalculateCost calculates the actual fee charged for the current transaction, based on gas used.
// For non-fee transactions it is implicitly used in GenericTxSystem.
// For fee transactions this function is NOT used in this module.
func (f *FeeCreditModule) CalculateCost(gasUsed uint64) uint64 {
	// in feeless mode all transactions are "free"
	if f.feelessMode {
		return 0
	}
	// in normal mode all transactions cost at least 1 tema
	cost := (gasUsed + feeModule.GasUnitsPerTema/2) / feeModule.GasUnitsPerTema
	if cost == 0 {
		cost = 1
	}
	return cost
}

func (f *FeeCreditModule) BuyGas(maxTxCost uint64) uint64 {
	return maxTxCost * feeModule.GasUnitsPerTema
}

func (f *FeeCreditModule) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		permissioned.TransactionTypeSetFeeCredit:    txtypes.NewTxHandler[permissioned.SetFeeCreditAttributes, permissioned.SetFeeCreditAuthProof](f.validateSetFC, f.executeSetFC),
		permissioned.TransactionTypeDeleteFeeCredit: txtypes.NewTxHandler[permissioned.DeleteFeeCreditAttributes, permissioned.DeleteFeeCreditAuthProof](f.validateDeleteFC, f.executeDeleteFC),
	}
}

func (f *FeeCreditModule) IsFeeCreditTx(tx *types.TransactionOrder) bool {
	return permissioned.IsFeeCreditTx(tx)
}

func (f *FeeCreditModule) IsCredible(exeCtx txtypes.ExecutionContext, tx *types.TransactionOrder) error {
	return f.feeBalanceValidator.IsCredible(exeCtx, tx)
}

func (f *FeeCreditModule) IsPermissionedMode() bool {
	return true
}

func (f *FeeCreditModule) IsFeelessMode() bool {
	return f.feelessMode
}
