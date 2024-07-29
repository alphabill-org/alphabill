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
	}

	FeeBalanceValidator interface {
		IsCredible(exeCtx ExecutionContext, tx *types.TransactionOrder) error
	}

	FeeTxVerifier interface {
		IsFeeCreditTx(tx *types.TransactionOrder) bool
	}
)
