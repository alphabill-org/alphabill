package domain

import (
	"bytes"

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
		Nonce        *uint256.Int
		TargetBearer Predicate
		TargetValue  uint64
	}

	BillSplit struct {
		Amount       uint64
		TargetBearer Predicate
		TargetValue  uint64
		Backlink     []byte
	}

	Swap struct {
		OwnerCondition     Predicate
		BillIdentifiers    []*uint256.Int
		DustTransferOrders []*DustTransfer
		Proofs             []*LedgerProof
		TargetValue        uint64
	}
)

func (s *Swap) Normalise() []byte {
	var bb bytes.Buffer
	bb.Write(s.OwnerCondition)
	for _, bi := range s.BillIdentifiers {
		if bi != nil {
			biBytes := bi.Bytes32()
			bb.Write(biBytes[:])
		}
	}

	for _, dt := range s.DustTransferOrders {
		if dt != nil {
			bb.Write(dt.Normalise())
		}
	}

	for _, lp := range s.Proofs {
		if lp != nil {
			bb.Write(lp.Normalise())
		}
	}

	bb.Write(Uint64ToBytes(s.TargetValue))
	return bb.Bytes()
}

func (b *BillSplit) Normalise() []byte {
	var bb bytes.Buffer
	bb.Write(Uint64ToBytes(b.Amount))
	bb.Write(b.TargetBearer)
	bb.Write(Uint64ToBytes(b.TargetValue))
	bb.Write(b.Backlink)
	return bb.Bytes()
}

func (d *DustTransfer) Normalise() []byte {
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
	bb.Write(d.TargetBearer)
	bb.Write(Uint64ToBytes(d.TargetValue))
	return bb.Bytes()
}

func (b *BillTransfer) Normalise() []byte {
	var bb bytes.Buffer
	bb.Write(b.NewBearer)
	bb.Write(b.Backlink)
	return bb.Bytes()
}
