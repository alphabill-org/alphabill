package transaction

import (
	"bytes"
	"crypto"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"github.com/holiman/uint256"
)

const protobufTypeUrlPrefix = "type.googleapis.com/rpc."

type wrapper struct {
	transaction *transaction.Transaction
	hashFunc    crypto.Hash
	hashValue   []byte
}

func (w *wrapper) UnitID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(w.transaction.UnitId)
}

func (w *wrapper) Timeout() uint64 {
	return w.transaction.Timeout
}

func (w *wrapper) SystemID() []byte {
	return w.transaction.SystemId
}

func (w *wrapper) OwnerProof() []byte {
	return w.transaction.OwnerProof
}

func (w *wrapper) sigBytes(b bytes.Buffer) {
	b.Write(w.transaction.SystemId)
	b.Write(w.transaction.UnitId)
	b.Write(util.Uint64ToBytes(w.transaction.Timeout))
}

func (w *wrapper) addTransactionFieldsToHasher(hasher hash.Hash) {
	hasher.Write(w.transaction.SystemId)
	hasher.Write(w.transaction.UnitId)
	hasher.Write(w.transaction.OwnerProof)
	hasher.Write(util.Uint64ToBytes(w.transaction.Timeout))
}

func (w *wrapper) hashComputed(hashFunc crypto.Hash) bool {
	return w.hashFunc == hashFunc && w.hashValue != nil
}
