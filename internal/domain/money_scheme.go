package domain

import (
	"github.com/holiman/uint256"
)

type (
	BillTransfer struct {
		NewBearer Predicate
		Backlink  []byte
	}

	DustTransfer struct {
		NewBearer    Predicate
		Backlink     []byte
		Nonce        uint256.Int
		TargetBearer Predicate
		TargetValue  uint32
	}

	BillSplit struct {
		Amount       uint32
		TargetBearer Predicate
		TargetValue  uint32
		Backlink     []byte
	}

	Swap struct {
		OwnerCondition     Predicate
		BillIdentifiers    []uint256.Int
		DustTransferOrders []DustTransfer
		Proofs             []LedgerProof
		TargetValue        uint32
	}
)
