package program

import (
	"bytes"
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

var typeURLPCall = "type.googleapis.com/rpc.PCallAttributes"
var typeURLPDeploy = "type.googleapis.com/rpc.PDeployAttributes"

type PCallTransactionOrder struct {
	attributes *PCallAttributes
	txOrder    *txsystem.Transaction
	hashFunc   crypto.Hash
	hashValue  []byte
}

type PDeployTransactionOrder struct {
	attributes *PDeployAttributes
	txOrder    *txsystem.Transaction
	hashFunc   crypto.Hash
	hashValue  []byte
}

func (s *PCallTransactionOrder) SystemID() []byte {
	return s.txOrder.SystemId
}

func (s *PCallTransactionOrder) UnitID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(s.txOrder.UnitId)
}

func (s *PCallTransactionOrder) Timeout() uint64 {
	return s.txOrder.Timeout()
}

func (s *PCallTransactionOrder) OwnerProof() []byte {
	return s.txOrder.OwnerProof
}

func (s *PCallTransactionOrder) Hash(hashFunc crypto.Hash) []byte {
	if s.hashFunc == hashFunc && s.hashValue != nil {
		return s.hashValue
	}
	hasher := hashFunc.New()
	s.AddToHasher(hasher)
	s.hashValue = hasher.Sum(nil)
	s.hashFunc = hashFunc
	return s.hashValue
}

func (s *PCallTransactionOrder) AddToHasher(hasher hash.Hash) {
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

func (s *PCallTransactionOrder) SigBytes() []byte {
	var b bytes.Buffer
	b.Write(s.txOrder.SystemId)
	b.Write(s.txOrder.UnitId)
	if s.txOrder.ClientMetadata != nil {
		b.Write(s.txOrder.ClientMetadata.Bytes())
	}
	b.Write(s.attributes.Input)
	return b.Bytes()
}

func (s *PCallTransactionOrder) IsPrimary() bool {
	return true
}

func (s *PCallTransactionOrder) ToProtoBuf() *txsystem.Transaction {
	return s.txOrder
}

func (s *PCallTransactionOrder) TargetUnits(crypto.Hash) []*uint256.Int {
	return []*uint256.Int{s.UnitID()}
}

func (s *PCallTransactionOrder) SetServerMetadata(sm *txsystem.ServerMetadata) {
	s.ToProtoBuf().ServerMetadata = sm
	s.hashValue = nil
}

func (s *PDeployTransactionOrder) SystemID() []byte {
	return s.txOrder.SystemId
}

func (s *PDeployTransactionOrder) UnitID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(s.txOrder.UnitId)
}

func (s *PDeployTransactionOrder) Timeout() uint64 {
	return s.txOrder.Timeout()
}

func (s *PDeployTransactionOrder) OwnerProof() []byte {
	return s.txOrder.OwnerProof
}

func (s *PDeployTransactionOrder) Hash(hashFunc crypto.Hash) []byte {
	if s.hashFunc == hashFunc && s.hashValue != nil {
		return s.hashValue
	}
	hasher := hashFunc.New()
	s.AddToHasher(hasher)
	s.hashValue = hasher.Sum(nil)
	s.hashFunc = hashFunc
	return s.hashValue
}

func (s *PDeployTransactionOrder) AddToHasher(hasher hash.Hash) {
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
	hasher.Write(s.attributes.InitData)
}

func (s *PDeployTransactionOrder) SigBytes() []byte {
	var b bytes.Buffer
	b.Write(s.txOrder.SystemId)
	b.Write(s.txOrder.UnitId)
	if s.txOrder.ClientMetadata != nil {
		b.Write(s.txOrder.ClientMetadata.Bytes())
	}
	b.Write(s.attributes.InitData)
	return b.Bytes()
}

func (s *PDeployTransactionOrder) IsPrimary() bool {
	return true
}

func (s *PDeployTransactionOrder) ToProtoBuf() *txsystem.Transaction {
	return s.txOrder
}

func (s *PDeployTransactionOrder) TargetUnits(crypto.Hash) []*uint256.Int {
	return []*uint256.Int{s.UnitID()}
}

func (s *PDeployTransactionOrder) SetServerMetadata(sm *txsystem.ServerMetadata) {
	s.ToProtoBuf().ServerMetadata = sm
	s.hashValue = nil
}
