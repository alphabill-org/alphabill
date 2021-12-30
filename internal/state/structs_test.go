package state

import (
	"crypto"

	"github.com/holiman/uint256"
)

// Struct definitions that fulfill the interfaces

type (
	genericTx struct {
		unitId     *uint256.Int
		timeout    uint64
		ownerProof []byte
	}

	transfer struct {
		genericTx
		newBearer   []byte
		targetValue uint64
		backlink    []byte
	}

	split struct {
		genericTx
		amount         uint64
		targetBearer   []byte
		remainingValue uint64
		backlink       []byte
	}
)

func (t *genericTx) UnitId() *uint256.Int { return t.unitId }
func (t *genericTx) Timeout() uint64      { return t.timeout }
func (t *genericTx) OwnerProof() []byte   { return t.ownerProof }

func (t *transfer) NewBearer() []byte                { return t.newBearer }
func (t *transfer) TargetValue() uint64              { return t.targetValue }
func (t *transfer) Backlink() []byte                 { return t.backlink }
func (t *transfer) Hash(hashFunc crypto.Hash) []byte { panic("implement me") }

func (s split) Amount() uint64                         { return s.amount }
func (s split) TargetBearer() []byte                   { return s.targetBearer }
func (s split) RemainingValue() uint64                 { return s.remainingValue }
func (s split) Backlink() []byte                       { return s.backlink }
func (s split) Hash(hashFunc crypto.Hash) []byte       { panic("implement me") }
func (s split) HashPrndSh(hashFunc crypto.Hash) []byte { return []byte("prnd sh hash value") }
