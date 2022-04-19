package transaction

import (
	"bytes"
	"crypto"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/money"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"github.com/holiman/uint256"
)

type (
	transferWrapper struct {
		wrapper
		transfer *BillTransfer
	}

	transferDCWrapper struct {
		wrapper
		transferDC *TransferDC
	}

	billSplitWrapper struct {
		wrapper
		billSplit *BillSplit
	}

	swapWrapper struct {
		wrapper
		swap *Swap
		// The dust collector transfers, that also exist inside swap as generic Transaction
		dcTransfers []*transferDCWrapper
	}
)

// NewMoneyTx creates a new wrapper, returns an error if unknown transaction type is given as argument.
func NewMoneyTx(tx *transaction.Transaction) (transaction.GenericTransaction, error) {
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
	case protobufTypeUrlPrefix + "TransferDC":
		pb := &TransferDC{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &transferDCWrapper{
			wrapper:    wrapper{transaction: tx},
			transferDC: pb,
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
		for _, dtTx := range pb.DcTransfers {
			dt, err := NewMoneyTx(dtTx)
			if err != nil {
				return nil, errors.Wrap(err, "transfer DC wrapping failed")
			}
			dtw, ok := dt.(*transferDCWrapper)
			if !ok {
				return nil, errors.Errorf("transfer DC wrapper is invalid type: %T", dt)
			}
			swapWr.dcTransfers = append(swapWr.dcTransfers, dtw)
		}
		return swapWr, nil
	default:
		return nil, errors.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
	}
}

func (w *transferWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()
	w.wrapper.addTransactionFieldsToHasher(hasher)

	hasher.Write(w.transfer.NewBearer)
	hasher.Write(util.Uint64ToBytes(w.transfer.TargetValue))
	hasher.Write(w.transfer.Backlink)

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *transferDCWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()

	w.addToHasher(hasher)

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *transferDCWrapper) addToHasher(hasher hash.Hash) {
	w.wrapper.addTransactionFieldsToHasher(hasher)
	w.transferDC.addFieldsToHasher(hasher)
}

func (w *billSplitWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()
	w.wrapper.addTransactionFieldsToHasher(hasher)
	w.addAttributesToHasher(hasher)

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *billSplitWrapper) addAttributesToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(w.billSplit.Amount))
	hasher.Write(w.billSplit.TargetBearer)
	hasher.Write(util.Uint64ToBytes(w.billSplit.RemainingValue))
	hasher.Write(w.billSplit.Backlink)
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

	for _, dt := range w.dcTransfers {
		dt.addToHasher(hasher)
	}

	for _, p := range w.swap.Proofs {
		hasher.Write(p)
	}

	hasher.Write(util.Uint64ToBytes(w.swap.TargetValue))

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *transferWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.wrapper.sigBytes(b)
	b.Write(w.NewBearer())
	b.Write(util.Uint64ToBytes(w.TargetValue()))
	b.Write(w.Backlink())
	return b.Bytes()
}

func (w *transferDCWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.wrapper.sigBytes(b)
	b.Write(w.Nonce())
	b.Write(w.TargetBearer())
	b.Write(util.Uint64ToBytes(w.TargetValue()))
	b.Write(w.Backlink())
	return b.Bytes()
}

func (w *billSplitWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.wrapper.sigBytes(b)
	b.Write(util.Uint64ToBytes(w.Amount()))
	b.Write(w.TargetBearer())
	b.Write(util.Uint64ToBytes(w.RemainingValue()))
	b.Write(w.Backlink())
	return b.Bytes()
}

func (w *swapWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.wrapper.sigBytes(b)
	b.Write(w.OwnerCondition())
	for _, billId := range w.BillIdentifiers() {
		bytes32 := billId.Bytes32()
		b.Write(bytes32[:])
	}
	for _, dcTx := range w.DCTransfers() {
		b.Write(dcTx.SigBytes())
	}
	for _, proof := range w.Proofs() {
		b.Write(proof)
	}
	b.Write(util.Uint64ToBytes(w.TargetValue()))
	return b.Bytes()
}

func (x *TransferDC) addFieldsToHasher(hasher hash.Hash) {
	hasher.Write(x.Nonce)
	hasher.Write(x.TargetBearer)
	hasher.Write(util.Uint64ToBytes(x.TargetValue))
	hasher.Write(x.Backlink)
}

// State interfaces compatibility

func (w *transferWrapper) NewBearer() []byte   { return w.transfer.NewBearer }
func (w *transferWrapper) TargetValue() uint64 { return w.transfer.TargetValue }
func (w *transferWrapper) Backlink() []byte    { return w.transfer.Backlink }

func (w *transferDCWrapper) Nonce() []byte        { return w.transferDC.Nonce }
func (w *transferDCWrapper) TargetBearer() []byte { return w.transferDC.TargetBearer }
func (w *transferDCWrapper) TargetValue() uint64  { return w.transferDC.TargetValue }
func (w *transferDCWrapper) Backlink() []byte     { return w.transferDC.Backlink }

func (w *billSplitWrapper) Amount() uint64         { return w.billSplit.Amount }
func (w *billSplitWrapper) TargetBearer() []byte   { return w.billSplit.TargetBearer }
func (w *billSplitWrapper) RemainingValue() uint64 { return w.billSplit.RemainingValue }
func (w *billSplitWrapper) Backlink() []byte       { return w.billSplit.Backlink }
func (w *billSplitWrapper) HashForIdCalculation(hashFunc crypto.Hash) []byte {
	hasher := hashFunc.New()
	idBytes := w.UnitID().Bytes32()
	hasher.Write(idBytes[:])
	w.addAttributesToHasher(hasher)
	hasher.Write(util.Uint64ToBytes(w.Timeout()))
	return hasher.Sum(nil)
}

func (w *swapWrapper) OwnerCondition() []byte { return w.swap.OwnerCondition }
func (w *swapWrapper) Proofs() [][]byte       { return w.swap.Proofs }
func (w *swapWrapper) TargetValue() uint64    { return w.swap.TargetValue }
func (w *swapWrapper) DCTransfers() []money.TransferDC {
	var sdt []money.TransferDC
	for _, dt := range w.dcTransfers {
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
