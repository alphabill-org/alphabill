package txsystem

import (
	"bytes"
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const (
	TypeTransferFCOrder = "TransferFCOrder"

	protobufTypeUrlPrefix  = "type.googleapis.com/"
	typeURLTransferFCOrder = protobufTypeUrlPrefix + TypeTransferFCOrder
)

type (
	Wrapper struct {
		Transaction *Transaction
		hashFunc    crypto.Hash
		hashValue   []byte
	}

	TransferFCWrapper struct {
		Wrapper
		TransferFC *TransferFCOrder
	}

	//AddFCWrapper struct {
	//	Transaction *Transaction
	//	addFCOrder  *AddFCOrder
	//
	//	// The fee credit transfer, that also exist inside addFCOrder as generic Transaction
	//	// feeCreditTransfer []*transferDCWrapper
	//}

	//TransferFC interface {
	//	GenericTransaction
	//	Amount() uint64
	//	TargetSystemID() []byte
	//	TargetRecordID() []byte
	//	EarliestAdditionTime() uint64
	//	LatestAdditionTime() uint64
	//	Nonce() []byte
	//	Backlink() []byte
	//}
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
func (w *Wrapper) ToProtoBuf() *Transaction {
	return w.Transaction
}
func (w *Wrapper) IsPrimary() bool {
	return true
}
func (w *Wrapper) TargetUnits(_ crypto.Hash) []*uint256.Int {
	return []*uint256.Int{w.UnitID()}
}

func (w *Wrapper) sigBytes(b *bytes.Buffer) {
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
	w.sigBytes(&b)
	w.TransferFC.sigBytes(&b)
	return b.Bytes()
}

//// Transaction Tx specific interface methods
//func (w *TransferFCWrapper) Amount() uint64               { return w.TransferFC.Amount }
//func (w *TransferFCWrapper) TargetSystemID() []byte       { return w.TransferFC.TargetSystemIdentifier }
//func (w *TransferFCWrapper) TargetRecordID() []byte       { return w.TransferFC.TargetRecordId }
//func (w *TransferFCWrapper) EarliestAdditionTime() uint64 { return w.TransferFC.EarliestAdditionTime }
//func (w *TransferFCWrapper) LatestAdditionTime() uint64   { return w.TransferFC.LatestAdditionTime }
//func (w *TransferFCWrapper) Nonce() []byte                { return w.TransferFC.Nonce }
//func (w *TransferFCWrapper) Backlink() []byte             { return w.TransferFC.Backlink }

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
