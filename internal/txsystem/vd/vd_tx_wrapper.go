package vd

// import (
// 	"bytes"
// 	"crypto"
// 	"fmt"
// 	"hash"

// 	"github.com/alphabill-org/alphabill/internal/txsystem"
// 	fc "github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
// 	"github.com/holiman/uint256"
// )

// const (
// 	TypeURLRegisterDataAttributes = "type.googleapis.com/alphabill.vd.v1.RegisterDataAttributes"
// )

// type (
// 	RegisterData interface {
// 		txsystem.GenericTransaction
// 		RegisteredData() []byte
// 	}
// )

// type (
// 	wrapper struct {
// 		transaction *txsystem.Transaction
// 		hashFunc    crypto.Hash
// 		hashValue   []byte
// 	}

// 	registerDataWrapper struct {
// 		wrapper
// 		registerData *RegisterDataAttributes
// 	}
// )

// func NewVDTx(systemID []byte, tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
// 	if !bytes.Equal(systemID, tx.GetSystemId()) {
// 		return nil, fmt.Errorf("transaction has invalid system identifier %X, expected %X", tx.GetSystemId(), systemID)
// 	}
// 	feeTx, err := fc.NewFeeCreditTx(tx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if feeTx != nil {
// 		return feeTx, nil
// 	}

// 	switch tx.TransactionAttributes.TypeUrl {
// 	case TypeURLRegisterDataAttributes:
// 		return convertRegisterData(tx)
// 	default:
// 		return nil, fmt.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
// 	}
// }

// func convertRegisterData(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
// 	return &registerDataWrapper{
// 		wrapper: wrapper{transaction: tx},
// 		registerData: &RegisterDataAttributes{},
// 	}, nil
// }

// func (w *registerDataWrapper) Hash(hashFunc crypto.Hash) []byte {
// 	if w.wrapper.hashComputed(hashFunc) {
// 		return w.wrapper.hashValue
// 	}
// 	hasher := hashFunc.New()
// 	w.AddToHasher(hasher)

// 	w.wrapper.hashValue = hasher.Sum(nil)
// 	w.wrapper.hashFunc = hashFunc
// 	return w.wrapper.hashValue
// }

// func (w *registerDataWrapper) AddToHasher(hasher hash.Hash) {
// 	w.wrapper.addTransactionFieldsToHasher(hasher)
// }

// func (w *registerDataWrapper) SigBytes() []byte {
// 	var b bytes.Buffer
// 	w.wrapper.sigBytes(&b)
// 	return b.Bytes()
// }

// func (w *registerDataWrapper) TargetUnits(_ crypto.Hash) []*uint256.Int {
// 	return []*uint256.Int{w.UnitID()}
// }

// func (w *registerDataWrapper) RegisteredData() []byte {
// 	return w.wrapper.transaction.UnitId
// }

// func (w *wrapper) UnitID() *uint256.Int {
// 	return uint256.NewInt(0).SetBytes(w.transaction.UnitId)
// }

// func (w *wrapper) Timeout() uint64 {
// 	return w.transaction.Timeout()
// }

// func (w *wrapper) SystemID() []byte {
// 	return w.transaction.SystemId
// }

// func (w *wrapper) OwnerProof() []byte {
// 	return w.transaction.OwnerProof
// }

// func (w *wrapper) ToProtoBuf() *txsystem.Transaction {
// 	return w.transaction
// }

// func (w *wrapper) IsPrimary() bool {
// 	return true
// }

// func (w *wrapper) SetServerMetadata(sm *txsystem.ServerMetadata) {
// 	w.ToProtoBuf().ServerMetadata = sm
// 	w.resetHasher()
// }

// func (w *wrapper) resetHasher() {
// 	w.hashValue = nil
// }

// func (w *wrapper) sigBytes(b *bytes.Buffer) {
// 	b.Write(w.transaction.SystemId)
// 	b.Write(w.transaction.UnitId)
// 	if w.transaction.ClientMetadata != nil {
// 		b.Write(w.transaction.ClientMetadata.Bytes())
// 	}
// }

// func (w *wrapper) addTransactionFieldsToHasher(hasher hash.Hash) {
// 	hasher.Write(w.transaction.SystemId)
// 	hasher.Write(w.transaction.UnitId)
// 	hasher.Write(w.transaction.OwnerProof)
// 	hasher.Write(w.transaction.FeeProof)
// 	if w.transaction.ClientMetadata != nil {
// 		hasher.Write(w.transaction.ClientMetadata.Bytes())
// 	}
// 	if w.transaction.ServerMetadata != nil {
// 		hasher.Write(w.transaction.ServerMetadata.Bytes())
// 	}
// }

// func (w *wrapper) hashComputed(hashFunc crypto.Hash) bool {
// 	return w.hashFunc == hashFunc && w.hashValue != nil
// }
