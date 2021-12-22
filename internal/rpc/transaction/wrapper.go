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

	billSplitWrapper struct {
		wrapper
		billSplit *BillSplit
	}

	swapWrapper struct {
		wrapper
		swap *Swap
		// The dust transfers, that also exist inside swap as generic Transaction
		dustTransfers []*dustTransferWrapper
	}
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
	case protobufTypeUrlPrefix + "BillSplit":
		pb := &BillSplit{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &billSplitWrapper{
			wrapper:   wrapper{transaction: tx},
			billSplit: pb,
		}, nil
	case protobufTypeUrlPrefix + "Swap":
		pb := &Swap{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		swapWr := &swapWrapper{
			wrapper: wrapper{transaction: tx},
			swap:    pb,
		}
		for _, dtTx := range pb.DustTransfers {
			dt, err := New(dtTx)
			if err != nil {
				return nil, errors.Wrap(err, "dust transfer wrapping failed")
			}
			dtw, ok := dt.(*dustTransferWrapper)
			if !ok {
				return nil, errors.Errorf("dust transfer wrapper is invalid type: %T", dt)
			}
			swapWr.dustTransfers = append(swapWr.dustTransfers, dtw)
		}
		return swapWr, nil
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

	w.addToHasher(hasher)

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *dustTransferWrapper) addToHasher(hasher hash.Hash) {
	w.wrapper.addTransactionFieldsToHasher(hasher)
	w.dustTransfer.addFieldsToHasher(hasher)
}

func (w *billSplitWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()
	w.wrapper.addTransactionFieldsToHasher(hasher)

	hasher.Write(Uint64ToBytes(w.billSplit.Amount))
	hasher.Write(w.billSplit.TargetBearer)
	hasher.Write(Uint64ToBytes(w.billSplit.RemainingValue))
	hasher.Write(w.billSplit.Backlink)

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *swapWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()
	w.wrapper.addTransactionFieldsToHasher(hasher)

	hasher.Write(w.swap.OwnerCondition)
	for _, bi := range w.swap.BillIdentifiers {
		hasher.Write(bi)
	}

	for _, dt := range w.dustTransfers {
		dt.addToHasher(hasher)
	}

	for _, p := range w.swap.Proofs {
		hasher.Write(p)
	}

	hasher.Write(Uint64ToBytes(w.swap.TargetValue))

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *wrapper) addTransactionFieldsToHasher(hasher hash.Hash) {
	hasher.Write(w.transaction.UnitId)
	hasher.Write(w.transaction.OwnerProof)
	hasher.Write(Uint64ToBytes(w.transaction.Timeout))
}

func (x *DustTransfer) addFieldsToHasher(hasher hash.Hash) {
	hasher.Write(x.NewBearer)
	hasher.Write(x.Backlink)
	hasher.Write(x.Nonce)
	hasher.Write(Uint64ToBytes(x.TargetValue))
}

func (w *wrapper) hashComputed(hashFunc crypto.Hash) bool {
	return w.hashFunc == hashFunc && w.hashValue != nil
}

// State interfaces compatibility

func (w *transferWrapper) NewBearer() []byte   { return w.transfer.NewBearer }
func (w *transferWrapper) Backlink() []byte    { return w.transfer.Backlink }
func (w *transferWrapper) TargetValue() uint64 { return w.transfer.TargetValue }

func (w *dustTransferWrapper) NewBearer() []byte   { return w.dustTransfer.NewBearer }
func (w *dustTransferWrapper) Backlink() []byte    { return w.dustTransfer.Backlink }
func (w *dustTransferWrapper) Nonce() []byte       { return w.dustTransfer.Nonce }
func (w *dustTransferWrapper) TargetValue() uint64 { return w.dustTransfer.TargetValue }

func (w *billSplitWrapper) Amount() uint64         { return w.billSplit.Amount }
func (w *billSplitWrapper) TargetBearer() []byte   { return w.billSplit.TargetBearer }
func (w *billSplitWrapper) RemainingValue() uint64 { return w.billSplit.RemainingValue }
func (w *billSplitWrapper) Backlink() []byte       { return w.billSplit.Backlink }

func (w *swapWrapper) OwnerCondition() []byte { return w.swap.OwnerCondition }
func (w *swapWrapper) Proofs() [][]byte       { return w.swap.Proofs }
func (w *swapWrapper) TargetValue() uint64    { return w.swap.TargetValue }
func (w *swapWrapper) DustTransfers() []state.DustTransfer {
	var sdt []state.DustTransfer
	for _, dt := range w.dustTransfers {
		sdt = append(sdt, dt)
	}
	return sdt
}
func (w *swapWrapper) BillIdentifiers() []*uint256.Int {
	var billIds []*uint256.Int
	for _, biBytes := range w.swap.BillIdentifiers {
		billIds = append(billIds, uint256.NewInt(0).SetBytes(biBytes))
	}
	return billIds
}
