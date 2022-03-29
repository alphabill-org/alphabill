package money

import (
	"crypto"
	"github.com/holiman/uint256"
)

// Struct definitions that fulfill the interfaces

type (
	genericTx struct {
		systemID   []byte
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

	transferDC struct {
		genericTx
		nonce        []byte
		targetBearer []byte
		targetValue  uint64
		backlink     []byte
	}

	split struct {
		genericTx
		amount         uint64
		targetBearer   []byte
		remainingValue uint64
		backlink       []byte
	}

	swap struct {
		genericTx
		ownerCondition  []byte
		billIdentifiers []*uint256.Int
		dcTransfers     []TransferDC
		proofs          [][]byte
		targetValue     uint64
	}
)

func (t *genericTx) SystemID() []byte     { return t.systemID }
func (t *genericTx) UnitId() *uint256.Int { return t.unitId }
func (t *genericTx) Timeout() uint64      { return t.timeout }
func (t *genericTx) OwnerProof() []byte   { return t.ownerProof }

func (t *transfer) NewBearer() []byte         { return t.newBearer }
func (t *transfer) TargetValue() uint64       { return t.targetValue }
func (t *transfer) Backlink() []byte          { return t.backlink }
func (t *transfer) Hash(_ crypto.Hash) []byte { return []byte("transfer hash") }

func (t *transferDC) Nonce() []byte                    { return t.nonce }
func (t *transferDC) TargetBearer() []byte             { return t.targetBearer }
func (t *transferDC) TargetValue() uint64              { return t.targetValue }
func (t *transferDC) Backlink() []byte                 { return t.backlink }
func (t *transferDC) Hash(hashFunc crypto.Hash) []byte { return []byte("transferDC hash") }

func (s split) Amount() uint64                   { return s.amount }
func (s split) TargetBearer() []byte             { return s.targetBearer }
func (s split) RemainingValue() uint64           { return s.remainingValue }
func (s split) Backlink() []byte                 { return s.backlink }
func (s split) Hash(hashFunc crypto.Hash) []byte { return []byte("split hash") }
func (s split) HashForIdCalculation(hashFunc crypto.Hash) []byte {
	return []byte("hash value for usage in sameShardId function")
}

func (s *swap) OwnerCondition() []byte          { return s.ownerCondition }
func (s *swap) BillIdentifiers() []*uint256.Int { return s.billIdentifiers }
func (s *swap) DCTransfers() []TransferDC       { return s.dcTransfers }
func (s *swap) Proofs() [][]byte                { return s.proofs }
func (s *swap) TargetValue() uint64             { return s.targetValue }
func (s *swap) Hash(_ crypto.Hash) []byte       { return []byte("swap hash") }
