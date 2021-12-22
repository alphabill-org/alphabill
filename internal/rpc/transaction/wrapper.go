package transaction

import (
	"crypto"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"

	"github.com/holiman/uint256"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

const protobufTypeUrlPrefix = "type.googleapis.com/rpc."

type (
	GenericTransaction interface {
		UnitId() *uint256.Int
		Timeout() uint64
		OwnerProof() []byte
		Hash(hashFunc crypto.Hash) []byte
	}

	wrapper struct {
		transaction *Transaction
		hashFunc    crypto.Hash
		hashValue   []byte
	}

	transferWrapper struct {
		wrapper
		transfer *BillTransfer
	}

	dustTransferWrapper struct {
		wrapper
		dustTransfer *DustTransfer
	}
	// TODO other transfer types
)

// New creates a new wrapper, returns an error if unknown transaction type is given as argument.
func New(tx *Transaction) (GenericTransaction, error) {
	switch tx.TransactionAttributes.TypeUrl {
	case protobufTypeUrlPrefix + "BillTransfer":
		pb := &BillTransfer{}
		// This is slow operation, involves reflection. Would be good to do this only once.
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &transferWrapper{
			wrapper:  wrapper{transaction: tx},
			transfer: pb,
		}, nil
	case protobufTypeUrlPrefix + "DustTransfer":
		pb := &DustTransfer{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &dustTransferWrapper{
			wrapper:      wrapper{transaction: tx},
			dustTransfer: pb,
		}, nil
	default:
		return nil, errors.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
	}
}

// GeneralTransaction interface

func (w *wrapper) UnitId() *uint256.Int {
	return uint256.NewInt(0).SetBytes(w.transaction.UnitId)
}

func (w *wrapper) Timeout() uint64 {
	return w.transaction.Timeout
}

func (w *wrapper) OwnerProof() []byte {
	return w.transaction.OwnerProof
}

func (w *transferWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()
	w.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(w.transfer.NewBearer)
	hasher.Write(w.transfer.Backlink)
	hasher.Write(Uint64ToBytes(w.transfer.TargetValue))
	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *dustTransferWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()
	w.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(w.dustTransfer.NewBearer)
	hasher.Write(w.dustTransfer.Backlink)
	hasher.Write(w.dustTransfer.Nonce)
	hasher.Write(Uint64ToBytes(w.dustTransfer.TargetValue))
	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *wrapper) addTransactionFieldsToHasher(hasher hash.Hash) {
	hasher.Write(w.transaction.UnitId)
	hasher.Write(w.transaction.OwnerProof)
	hasher.Write(Uint64ToBytes(w.transaction.Timeout))
}

func (w *wrapper) hashComputed(hashFunc crypto.Hash) bool {
	return w.hashFunc == hashFunc && w.hashValue != nil
}

// State interfaces compatibility

func (w *transferWrapper) NewBearer() []byte           { return w.transfer.NewBearer }
func (w *transferWrapper) Backlink() []byte            { return w.transfer.Backlink }
func (w *transferWrapper) TargetValue() uint64         { return w.transfer.TargetValue }
func (w *transferWrapper) Type() state.TransactionType { return state.TypeTransfer }

func (w *dustTransferWrapper) NewBearer() []byte           { return w.dustTransfer.NewBearer }
func (w *dustTransferWrapper) Backlink() []byte            { return w.dustTransfer.Backlink }
func (w *dustTransferWrapper) Nonce() []byte               { return w.dustTransfer.Nonce }
func (w *dustTransferWrapper) TargetValue() uint64         { return w.dustTransfer.TargetValue }
func (w *dustTransferWrapper) Type() state.TransactionType { return state.TypeDCTransfer }
