package transaction

import (
	"bytes"
	"crypto"
	"encoding/base64"
	hasherUtil "gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"github.com/holiman/uint256"
	"hash"
)

type (
	GenericTransaction interface {
		SystemID() []byte
		UnitID() *uint256.Int
		IDHash() string
		Timeout() uint64
		OwnerProof() []byte
		Hash(hashFunc crypto.Hash) []byte
		SigBytes() []byte
	}

	wrapper struct {
		transaction *Transaction
		hashFunc    crypto.Hash
		hashValue   []byte
	}
)

// GeneralTransaction interface

func (w *wrapper) UnitID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(w.transaction.UnitId)
}

func (w *wrapper) IDHash() string {
	idHash := hasherUtil.Sum256(w.UnitID().Bytes())
	return base64.StdEncoding.EncodeToString(idHash)
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
	b.Write(Uint64ToBytes(w.transaction.Timeout))
}

func (w *wrapper) addTransactionFieldsToHasher(hasher hash.Hash) {
	hasher.Write(w.transaction.SystemId)
	hasher.Write(w.transaction.UnitId)
	hasher.Write(w.transaction.OwnerProof)
	hasher.Write(Uint64ToBytes(w.transaction.Timeout))
}

func (w *wrapper) hashComputed(hashFunc crypto.Hash) bool {
	return w.hashFunc == hashFunc && w.hashValue != nil
}
