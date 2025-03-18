package fc

import (
	"math"

	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

var _ txtypes.Module = (*NoFeeHandling)(nil)

type NoFeeHandling struct{}

func NewNoFeeCreditModule() *NoFeeHandling {
	return &NoFeeHandling{}
}

func (f *NoFeeHandling) CalculateCost(_ uint64) uint64 {
	return 0
}

func (f *NoFeeHandling) BuyGas(_ uint64) uint64 {
	return math.MaxUint64
}

func (f *NoFeeHandling) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{}
}

func (f *NoFeeHandling) IsCredible(_ txtypes.ExecutionContext, _ *types.TransactionOrder) error {
	return nil
}

func (f *NoFeeHandling) IsFeeCreditTx(tx *types.TransactionOrder) bool {
	return false
}

func (f *NoFeeHandling) IsPermissionedMode() bool {
	return false
}

func (f *NoFeeHandling) IsFeelessMode() bool {
	return true
}

func (f *NoFeeHandling) FeeCreditRecordUnitType() uint32 {
	return 0
}
