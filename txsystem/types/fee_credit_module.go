package types

import (
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	FeeCreditModule interface {
		Module
		FeeCalculation
		FeeBalanceValidator
		FeeTxVerifier

		IsPermissionedMode() bool
		IsFeelessMode() bool
		FeeCreditRecordUnitType() uint32
	}

	FeeBalanceValidator interface {
		IsCredible(exeCtx ExecutionContext, tx *types.TransactionOrder) error
	}

	FeeTxVerifier interface {
		IsFeeCreditTx(tx *types.TransactionOrder) bool
	}

	Orchestration interface {
		TrustBase(epoch uint64) (types.RootTrustBase, error)
	}
)
