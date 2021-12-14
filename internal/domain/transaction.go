package domain

import (
	"github.com/holiman/uint256"
)

type (
	TransactionOrder struct {
		TransactionId         *uint256.Int
		TransactionAttributes interface{}
		Timeout               uint64
		OwnerProof            Predicate
	}
)
