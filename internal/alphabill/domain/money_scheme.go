package domain

import (
	"bytes"

	"github.com/holiman/uint256"
)

const (
	TypeTransfer TransactionType = iota
	TypeDCTransfer
	TypeSplit
	TypeSwap
)

type (
	TransactionType int

	BillTransfer struct {
		NewBearer Predicate
		Backlink  []byte
		Amount    uint64
	}

	DustTransfer struct {
		NewBearer   Predicate
		Backlink    []byte
		Nonce       *uint256.Int
		TargetValue uint64
	}

	BillSplit struct {
		Amount         uint64
		TargetBearer   Predicate
		RemainingValue uint64
		Backlink       []byte
	}

	Swap struct {
		BearerCondition    Predicate
		BillIdentifiers    []*uint256.Int
		DustTransferOrders []*DustTransfer
		//Proofs             []*LedgerProof
		TargetValue uint64
	}
)

func (s *Swap) Bytes() []byte {
	var bb bytes.Buffer
	bb.Write(s.BearerCondition)
	for _, bi := range s.BillIdentifiers {
		if bi != nil {
			biBytes := bi.Bytes32()
			bb.Write(biBytes[:])
		}
	}

	for _, dt := range s.DustTransferOrders {
		if dt != nil {
			bb.Write(dt.Bytes())
		}
	}

	//for _, lp := range s.Proofs {
	//	if lp != nil {
	//		bb.Write(lp.Bytes())
	//	}
	//}

	bb.Write(Uint64ToBytes(s.TargetValue))
	return bb.Bytes()
}

func (s *Swap) Value() uint64 {
	return s.TargetValue
}

func (s *Swap) OwnerCondition() []byte {
	return s.BearerCondition
}

func (b *BillSplit) Bytes() []byte {
	var bb bytes.Buffer
	bb.Write(Uint64ToBytes(b.Amount))
	bb.Write(b.TargetBearer)
	bb.Write(Uint64ToBytes(b.RemainingValue))
	bb.Write(b.Backlink)
	return bb.Bytes()
}

func (b *BillSplit) Value() uint64 {
	return b.RemainingValue
}

func (b *BillSplit) OwnerCondition() []byte {
	return b.TargetBearer
}

func (d *DustTransfer) Bytes() []byte {
	var bb bytes.Buffer
	bb.Write(d.NewBearer)
	bb.Write(d.Backlink)
	if d.Nonce != nil {
		nonceBytes := d.Nonce.Bytes32()
		bb.Write(nonceBytes[:])
	} else {
		nonceBytes := [32]byte{}
		bb.Write(nonceBytes[:])
	}
	bb.Write(Uint64ToBytes(d.TargetValue))
	return bb.Bytes()
}

func (d *DustTransfer) Value() uint64 {
	return d.TargetValue
}

func (d *DustTransfer) OwnerCondition() []byte {
	return d.NewBearer
}

func (b *BillTransfer) Bytes() []byte {
	var bb bytes.Buffer
	bb.Write(b.NewBearer)
	bb.Write(b.Backlink)
	return bb.Bytes()
}

func (b *BillTransfer) Value() uint64 {
	return b.Amount
}

func (b *BillTransfer) OwnerCondition() []byte {
	return b.NewBearer
}
