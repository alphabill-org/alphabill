package types

import "github.com/alphabill-org/alphabill-go-base/types"

type (
	FeeCreditModule interface {
		Module
		FeeCalculation
		FeeBalanceValidator
	}

	FeeBalanceValidator interface {
		IsCredible(exeCtx ExecutionContext, tx *types.TransactionOrder) error
	}
)
