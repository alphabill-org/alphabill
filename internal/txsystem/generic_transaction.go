package txsystem

import (
	"bytes"
	"crypto"
	"errors"
	"hash"
	"sync"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

type (
	GenericTransaction interface {
		SystemID() []byte
		UnitID() *uint256.Int
		Timeout() uint64
		OwnerProof() []byte
		Hash(hashFunc crypto.Hash) []byte
		AddToHasher(hasher hash.Hash)
		ToProtoBuf() *Transaction
		SigBytes() []byte
		IsPrimary() bool
		TargetUnits(hashFunc crypto.Hash) []*uint256.Int
	}

	// DefaultGenericTransaction is a default implementation of GenericTransaction interface. NB! Only suitable for
	// transactions that contains systemID, unitID, timeout, and OwnerProof. If the transaction contains more attributes
	// then the transaction system developer must provide its own implementation.
	DefaultGenericTransaction struct {
		transaction *Transaction
		hashFunc    crypto.Hash
		hashValue   []byte
		mutex       sync.Mutex
	}
)

// NewDefaultGenericTransaction creates a new DefaultGenericTransaction from the given Transaction.
func NewDefaultGenericTransaction(transaction *Transaction) (GenericTransaction, error) {
	if transaction == nil {
		return nil, errors.New("input transaction is nil")
	}
	return &DefaultGenericTransaction{
		transaction: transaction,
	}, nil
}

func (d *DefaultGenericTransaction) SystemID() []byte {
	return d.transaction.SystemId
}

func (d *DefaultGenericTransaction) UnitID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(d.transaction.UnitId)
}

func (d *DefaultGenericTransaction) Timeout() uint64 {
	return d.transaction.Timeout
}

func (d *DefaultGenericTransaction) OwnerProof() []byte {
	return d.transaction.OwnerProof
}

func (d *DefaultGenericTransaction) sigBytes(b *bytes.Buffer) {
	b.Write(d.transaction.SystemId)
	b.Write(d.transaction.UnitId)
	b.Write(util.Uint64ToBytes(d.transaction.Timeout))
}

func (d *DefaultGenericTransaction) Hash(hashFunc crypto.Hash) []byte {
	if d.hashFunc == hashFunc && d.hashValue != nil {
		return d.hashValue
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	hasher := hashFunc.New()
	hasher.Write(d.transaction.Bytes())
	d.hashValue = hasher.Sum(nil)
	return d.hashValue
}

func (d *DefaultGenericTransaction) AddToHasher(hasher hash.Hash) {
	hasher.Write(d.transaction.Bytes())
}

func (d *DefaultGenericTransaction) ToProtoBuf() *Transaction {
	return d.transaction
}

func (d *DefaultGenericTransaction) SigBytes() []byte {
	var b bytes.Buffer
	d.sigBytes(&b)
	return b.Bytes()
}

func (d *DefaultGenericTransaction) IsPrimary() bool {
	return true
}

func (d *DefaultGenericTransaction) TargetUnits(_ crypto.Hash) []*uint256.Int {
	return []*uint256.Int{d.UnitID()}
}
