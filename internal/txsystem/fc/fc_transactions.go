package fc

import (
	"bytes"
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const (
	TypeTransferFCOrder = "TransferFCOrder"
	TypeAddFCOrder      = "AddFCOrder"
	TypeCloseFCOrder    = "CloseFCOrder"
	TypeReclaimFCOrder  = "ReclaimFCOrder"

	protobufTypeUrlPrefix  = "type.googleapis.com/"
	typeURLTransferFCOrder = protobufTypeUrlPrefix + TypeTransferFCOrder
	typeURLAddFCOrder      = protobufTypeUrlPrefix + TypeAddFCOrder
	typeURLCloseFCOrder    = protobufTypeUrlPrefix + TypeCloseFCOrder
	typeURLReclaimFCOrder  = protobufTypeUrlPrefix + TypeReclaimFCOrder
)

type (
	Wrapper struct {
		Transaction *txsystem.Transaction
		hashFunc    crypto.Hash
		hashValue   []byte
	}

	TransferFCWrapper struct {
		Wrapper
		TransferFC *TransferFCOrder
	}

	AddFCWrapper struct {
		Wrapper
		AddFC *AddFCOrder

		// The fee credit transfer that also exist inside addFCOrder as *txsystem.Transaction
		// needed to correctly serialize bytes
		feeCreditTransfer *TransferFCWrapper
	}

	CloseFCWrapper struct {
		Wrapper
		CloseFC *CloseFCOrder
	}

	ReclaimFCWrapper struct {
		Wrapper
		ReclaimFC *ReclaimFCOrder

		// The "close fee credit" transfer that also exist inside reclaimFCOrder as *txsystem.Transaction
		// needed to correctly serialize bytes
		closeFeeCreditTransfer *CloseFCWrapper
	}
)

// GenericTransaction methods
func (w *Wrapper) SystemID() []byte {
	return w.Transaction.SystemId
}
func (w *Wrapper) UnitID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(w.Transaction.UnitId)
}
func (w *Wrapper) Timeout() uint64 {
	return w.Transaction.Timeout
}
func (w *Wrapper) OwnerProof() []byte {
	return w.Transaction.OwnerProof
}
func (w *Wrapper) ToProtoBuf() *txsystem.Transaction {
	return w.Transaction
}
func (w *Wrapper) IsPrimary() bool {
	return true
}
func (w *Wrapper) TargetUnits(_ crypto.Hash) []*uint256.Int {
	return []*uint256.Int{w.UnitID()}
}
func (w *Wrapper) transactionSigBytes(b *bytes.Buffer) {
	b.Write(w.Transaction.SystemId)
	b.Write(w.Transaction.UnitId)
	b.Write(util.Uint64ToBytes(w.Transaction.Timeout))
}
func (w *Wrapper) addTransactionFieldsToHasher(hasher hash.Hash) {
	hasher.Write(w.Transaction.SystemId)
	hasher.Write(w.Transaction.UnitId)
	hasher.Write(w.Transaction.OwnerProof)
	hasher.Write(util.Uint64ToBytes(w.Transaction.Timeout))
}
func (w *Wrapper) hashComputed(hashFunc crypto.Hash) bool {
	return w.hashFunc == hashFunc && w.hashValue != nil
}

// GenericTransaction methods (transaction specific)
func (w *TransferFCWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.hashComputed(hashFunc) {
		return w.hashValue
	}
	hasher := hashFunc.New()
	w.AddToHasher(hasher)

	w.hashValue = hasher.Sum(nil)
	w.hashFunc = hashFunc
	return w.hashValue
}
func (w *TransferFCWrapper) AddToHasher(hasher hash.Hash) {
	w.Wrapper.addTransactionFieldsToHasher(hasher)
	w.TransferFC.addFieldsToHasher(hasher)
}
func (w *TransferFCWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.transactionSigBytes(&b)
	w.TransferFC.sigBytes(&b)
	return b.Bytes()
}

func (w *AddFCWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.hashComputed(hashFunc) {
		return w.hashValue
	}
	hasher := hashFunc.New()
	w.AddToHasher(hasher)

	w.hashValue = hasher.Sum(nil)
	w.hashFunc = hashFunc
	return w.hashValue
}
func (w *AddFCWrapper) AddToHasher(hasher hash.Hash) {
	w.Wrapper.addTransactionFieldsToHasher(hasher)
	w.addFieldsToHasher(hasher)
}
func (w *AddFCWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.transactionSigBytes(&b)
	w.sigBytes(&b)
	return b.Bytes()
}
func (w *AddFCWrapper) addFieldsToHasher(hasher hash.Hash) {
	hasher.Write(w.AddFC.FeeCreditOwnerCondition)
	w.feeCreditTransfer.AddToHasher(hasher)
	w.AddFC.FeeCreditTransferProof.AddToHasher(hasher)
}
func (w *AddFCWrapper) sigBytes(b *bytes.Buffer) {
	b.Write(w.AddFC.FeeCreditOwnerCondition)
	b.Write(w.feeCreditTransfer.SigBytes())
	b.Write(w.AddFC.FeeCreditTransferProof.Bytes())
}

func (w *CloseFCWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.hashComputed(hashFunc) {
		return w.hashValue
	}
	hasher := hashFunc.New()
	w.AddToHasher(hasher)

	w.hashValue = hasher.Sum(nil)
	w.hashFunc = hashFunc
	return w.hashValue
}
func (w *CloseFCWrapper) AddToHasher(hasher hash.Hash) {
	w.Wrapper.addTransactionFieldsToHasher(hasher)
	w.CloseFC.addFieldsToHasher(hasher)
}
func (w *CloseFCWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.transactionSigBytes(&b)
	w.CloseFC.sigBytes(&b)
	return b.Bytes()
}

func (w *ReclaimFCWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.hashComputed(hashFunc) {
		return w.hashValue
	}
	hasher := hashFunc.New()
	w.AddToHasher(hasher)

	w.hashValue = hasher.Sum(nil)
	w.hashFunc = hashFunc
	return w.hashValue
}
func (w *ReclaimFCWrapper) AddToHasher(hasher hash.Hash) {
	w.Wrapper.addTransactionFieldsToHasher(hasher)
	w.addFieldsToHasher(hasher)
}
func (w *ReclaimFCWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.transactionSigBytes(&b)
	w.sigBytes(&b)
	return b.Bytes()
}
func (w *ReclaimFCWrapper) addFieldsToHasher(hasher hash.Hash) {
	w.closeFeeCreditTransfer.AddToHasher(hasher)
	w.ReclaimFC.CloseFeeCreditProof.AddToHasher(hasher)
	hasher.Write(w.ReclaimFC.Backlink)
}
func (w *ReclaimFCWrapper) sigBytes(b *bytes.Buffer) {
	b.Write(w.closeFeeCreditTransfer.SigBytes())
	b.Write(w.ReclaimFC.CloseFeeCreditProof.Bytes())
	b.Write(w.ReclaimFC.Backlink)
}

// Protobuf transaction struct methods
func (x *TransferFCOrder) addFieldsToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(x.Amount))
	hasher.Write(x.TargetSystemIdentifier)
	hasher.Write(x.TargetRecordId)
	hasher.Write(util.Uint64ToBytes(x.EarliestAdditionTime))
	hasher.Write(util.Uint64ToBytes(x.LatestAdditionTime))
	hasher.Write(x.Nonce)
	hasher.Write(x.Backlink)
}
func (x *TransferFCOrder) sigBytes(b *bytes.Buffer) {
	b.Write(util.Uint64ToBytes(x.Amount))
	b.Write(x.TargetSystemIdentifier)
	b.Write(x.TargetRecordId)
	b.Write(util.Uint64ToBytes(x.EarliestAdditionTime))
	b.Write(util.Uint64ToBytes(x.LatestAdditionTime))
	b.Write(x.Nonce)
	b.Write(x.Backlink)
}

func (x *CloseFCOrder) addFieldsToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(x.Amount))
	hasher.Write(x.TargetUnitId)
	hasher.Write(x.Nonce)
}
func (x *CloseFCOrder) sigBytes(b *bytes.Buffer) {
	b.Write(util.Uint64ToBytes(x.Amount))
	b.Write(x.TargetUnitId)
	b.Write(x.Nonce)
}
