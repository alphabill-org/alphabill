package sc

import (
	"bytes"
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

var typeURLSCall = "type.googleapis.com/rpc.SCallAttributes"

type SCallTransactionOrder struct {
	attributes *SCallAttributes
	txOrder    *txsystem.Transaction
	hashFunc   crypto.Hash
	hashValue  []byte
}

func (s *SCallTransactionOrder) SystemID() []byte {
	return s.txOrder.SystemId
}

func (s *SCallTransactionOrder) UnitID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(s.txOrder.UnitId)
}

func (s *SCallTransactionOrder) Timeout() uint64 {
	return s.txOrder.Timeout()
}

func (s *SCallTransactionOrder) OwnerProof() []byte {
	return s.txOrder.OwnerProof
}

func (s *SCallTransactionOrder) Hash(hashFunc crypto.Hash) []byte {
	if s.hashFunc == hashFunc && s.hashValue != nil {
		return s.hashValue
	}
	hasher := hashFunc.New()
	s.AddToHasher(hasher)
	s.hashValue = hasher.Sum(nil)
	s.hashFunc = hashFunc
	return s.hashValue
}

func (s *SCallTransactionOrder) AddToHasher(hasher hash.Hash) {
	hasher.Write(s.txOrder.SystemId)
	hasher.Write(s.txOrder.UnitId)
	hasher.Write(s.txOrder.OwnerProof)
	hasher.Write(s.txOrder.FeeProof)
	if s.txOrder.ClientMetadata != nil {
		hasher.Write(s.txOrder.ClientMetadata.Bytes())
	}
	if s.txOrder.ServerMetadata != nil {
		hasher.Write(s.txOrder.ServerMetadata.Bytes())
	}
	hasher.Write(s.attributes.Input)
}

func (s *SCallTransactionOrder) SigBytes() []byte {
	var b bytes.Buffer
	b.Write(s.txOrder.SystemId)
	b.Write(s.txOrder.UnitId)
	if s.txOrder.ClientMetadata != nil {
		b.Write(s.txOrder.ClientMetadata.Bytes())
	}
	b.Write(s.attributes.Input)
	return b.Bytes()
}

func (s *SCallTransactionOrder) IsPrimary() bool {
	return true
}

func (s *SCallTransactionOrder) ToProtoBuf() *txsystem.Transaction {
	return s.txOrder
}

func (s *SCallTransactionOrder) TargetUnits(crypto.Hash) []*uint256.Int {
	return []*uint256.Int{s.UnitID()}
}

func (s *SCallTransactionOrder) SetServerMetadata(sm *txsystem.ServerMetadata) {
	s.ToProtoBuf().ServerMetadata = sm
	s.hashValue = nil
}
