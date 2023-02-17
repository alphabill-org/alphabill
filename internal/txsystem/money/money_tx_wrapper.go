package money

import (
	"bytes"
	"crypto"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const (
	TypeTransferOrder   = "TransferOrder"
	TypeTransferDCOrder = "TransferDCOrder"
	TypeSplitOrder      = "SplitOrder"
	TypeSwapOrder       = "SwapOrder"

	protobufTypeUrlPrefix  = "type.googleapis.com/rpc."
	typeURLTransferOrder   = protobufTypeUrlPrefix + TypeTransferOrder
	typeURLTransferDCOrder = protobufTypeUrlPrefix + TypeTransferDCOrder
	typeURLSplitOrder      = protobufTypeUrlPrefix + TypeSplitOrder
	typeURLSwapOrder       = protobufTypeUrlPrefix + TypeSwapOrder
)

type (
	Transfer interface {
		txsystem.GenericTransaction
		NewBearer() []byte
		TargetValue() uint64
		Backlink() []byte
	}

	TransferDC interface {
		txsystem.GenericTransaction
		Nonce() []byte
		TargetBearer() []byte
		TargetValue() uint64
		Backlink() []byte
	}

	Split interface {
		txsystem.GenericTransaction
		Amount() uint64
		TargetBearer() []byte
		RemainingValue() uint64
		Backlink() []byte
		HashForIdCalculation(hashFunc crypto.Hash) []byte // Returns hash value for the sameShardId function
	}

	Swap interface {
		txsystem.GenericTransaction
		OwnerCondition() []byte
		BillIdentifiers() []*uint256.Int
		DCTransfers() []*transferDCWrapper
		Proofs() []*block.BlockProof
		TargetValue() uint64
	}
)

type (
	wrapper struct {
		transaction *txsystem.Transaction
		hashFunc    crypto.Hash
		hashValue   []byte
	}

	transferWrapper struct {
		wrapper
		transfer *TransferOrder
	}

	transferDCWrapper struct {
		wrapper
		transferDC *TransferDCOrder
	}

	billSplitWrapper struct {
		wrapper
		billSplit *SplitOrder
	}

	swapWrapper struct {
		wrapper
		swap *SwapOrder
		// The dust collector transfers, that also exist inside swap as generic Transaction
		dcTransfers []*transferDCWrapper
	}
)

var _ Swap = &swapWrapper{}

// NewMoneyTx creates a new wrapper, returns an error if unknown transaction type is given as argument.
func NewMoneyTx(systemID []byte, tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	if !bytes.Equal(systemID, tx.GetSystemId()) {
		return nil, fmt.Errorf("transaction has invalid system identifier %X, expected %X", tx.GetSystemId(), systemID)
	}
	feeTx, err := transactions.NewFeeCreditTx(tx)
	if err != nil {
		return nil, err
	}
	if feeTx != nil {
		return feeTx, nil
	}
	switch tx.TransactionAttributes.TypeUrl {
	case typeURLTransferOrder:
		return convertTransferTx(tx)
	case typeURLTransferDCOrder:
		return convertTransferDCTx(tx)
	case typeURLSplitOrder:
		return convertSplitTx(tx)
	case typeURLSwapOrder:
		return convertSwapDCTx(tx)
	default:
		return nil, fmt.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
	}
}

func convertSwapDCTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	pb := &SwapOrder{}
	err := tx.TransactionAttributes.UnmarshalTo(pb)
	if err != nil {
		return nil, err
	}
	swapWr := &swapWrapper{
		wrapper: wrapper{transaction: tx},
		swap:    pb,
	}
	for _, dtTx := range pb.DcTransfers {
		dt, err := convertTransferDCTx(dtTx)

		if err != nil {
			return nil, fmt.Errorf("transfer DC wrapping failed: %w", err)
		}
		dtw, ok := dt.(*transferDCWrapper)
		if !ok {
			return nil, fmt.Errorf("transfer DC wrapper is invalid type: %T", dt)
		}
		if !bytes.Equal(tx.SystemId, dt.SystemID()) {
			return nil, fmt.Errorf("transferDC transaction has invalid system identifier %X, expected %X", tx.GetSystemId(), dt.SystemID())
		}
		swapWr.dcTransfers = append(swapWr.dcTransfers, dtw)
	}
	return swapWr, nil
}

func convertSplitTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	pb := &SplitOrder{}
	err := tx.TransactionAttributes.UnmarshalTo(pb)
	if err != nil {
		return nil, err
	}
	return &billSplitWrapper{
		wrapper:   wrapper{transaction: tx},
		billSplit: pb,
	}, nil
}

func convertTransferDCTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	pb := &TransferDCOrder{}
	err := tx.TransactionAttributes.UnmarshalTo(pb)
	if err != nil {
		return nil, err
	}
	return &transferDCWrapper{
		wrapper:    wrapper{transaction: tx},
		transferDC: pb,
	}, nil
}

func convertTransferTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	pb := &TransferOrder{}
	// This is slow operation, involves reflection. Would be good to do this only once.
	err := tx.TransactionAttributes.UnmarshalTo(pb)
	if err != nil {
		return nil, err
	}
	return &transferWrapper{
		wrapper:  wrapper{transaction: tx},
		transfer: pb,
	}, nil
}

func (w *transferWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()
	w.AddToHasher(hasher)

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *transferWrapper) AddToHasher(hasher hash.Hash) {
	w.wrapper.addTransactionFieldsToHasher(hasher)

	hasher.Write(w.transfer.NewBearer)
	hasher.Write(util.Uint64ToBytes(w.transfer.TargetValue))
	hasher.Write(w.transfer.Backlink)
}

func (w *transferDCWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()
	w.AddToHasher(hasher)

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *transferDCWrapper) AddToHasher(hasher hash.Hash) {
	w.wrapper.addTransactionFieldsToHasher(hasher)
	w.transferDC.addFieldsToHasher(hasher)
}

func (w *billSplitWrapper) Hash(hashFunc crypto.Hash) []byte {
	if w.wrapper.hashComputed(hashFunc) {
		return w.wrapper.hashValue
	}
	hasher := hashFunc.New()
	w.AddToHasher(hasher)

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *billSplitWrapper) AddToHasher(hasher hash.Hash) {
	w.wrapper.addTransactionFieldsToHasher(hasher)
	w.addAttributesToHasher(hasher)
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
	w.AddToHasher(hasher)

	w.wrapper.hashValue = hasher.Sum(nil)
	w.wrapper.hashFunc = hashFunc
	return w.wrapper.hashValue
}

func (w *swapWrapper) AddToHasher(hasher hash.Hash) {
	w.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(w.swap.OwnerCondition)
	for _, bi := range w.swap.BillIdentifiers {
		hasher.Write(bi)
	}
	for _, dt := range w.dcTransfers {
		dt.AddToHasher(hasher)
	}
	for _, p := range w.swap.Proofs {
		p.AddToHasher(hasher)
	}
	hasher.Write(util.Uint64ToBytes(w.swap.TargetValue))
}

func (w *transferWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.wrapper.sigBytes(&b)
	b.Write(w.NewBearer())
	b.Write(util.Uint64ToBytes(w.TargetValue()))
	b.Write(w.Backlink())
	return b.Bytes()
}

func (w *transferDCWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.wrapper.sigBytes(&b)
	b.Write(w.Nonce())
	b.Write(w.TargetBearer())
	b.Write(util.Uint64ToBytes(w.TargetValue()))
	b.Write(w.Backlink())
	return b.Bytes()
}

func (w *billSplitWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.wrapper.sigBytes(&b)
	b.Write(util.Uint64ToBytes(w.Amount()))
	b.Write(w.TargetBearer())
	b.Write(util.Uint64ToBytes(w.RemainingValue()))
	b.Write(w.Backlink())
	return b.Bytes()
}

func (w *swapWrapper) SigBytes() []byte {
	var b bytes.Buffer
	w.wrapper.sigBytes(&b)
	b.Write(w.OwnerCondition())
	for _, billId := range w.BillIdentifiers() {
		bytes32 := billId.Bytes32()
		b.Write(bytes32[:])
	}
	for _, dcTx := range w.DCTransfers() {
		b.Write(dcTx.SigBytes())
		b.Write(dcTx.OwnerProof())
	}
	for _, proof := range w.Proofs() {
		b.Write(proof.Bytes())
	}
	b.Write(util.Uint64ToBytes(w.TargetValue()))
	return b.Bytes()
}

func (x *TransferDCOrder) addFieldsToHasher(hasher hash.Hash) {
	hasher.Write(x.Nonce)
	hasher.Write(x.TargetBearer)
	hasher.Write(util.Uint64ToBytes(x.TargetValue))
	hasher.Write(x.Backlink)
}

// State interfaces compatibility

func (w *transferWrapper) NewBearer() []byte   { return w.transfer.NewBearer }
func (w *transferWrapper) TargetValue() uint64 { return w.transfer.TargetValue }
func (w *transferWrapper) Backlink() []byte    { return w.transfer.Backlink }
func (w *transferWrapper) TargetUnits(_ crypto.Hash) []*uint256.Int {
	return []*uint256.Int{w.UnitID()}
}

func (w *transferDCWrapper) Nonce() []byte        { return w.transferDC.Nonce }
func (w *transferDCWrapper) TargetBearer() []byte { return w.transferDC.TargetBearer }
func (w *transferDCWrapper) TargetValue() uint64  { return w.transferDC.TargetValue }
func (w *transferDCWrapper) Backlink() []byte     { return w.transferDC.Backlink }
func (w *transferDCWrapper) TargetUnits(_ crypto.Hash) []*uint256.Int {
	return []*uint256.Int{w.UnitID()}
}

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
func (w *billSplitWrapper) TargetUnits(hashFunc crypto.Hash) []*uint256.Int {
	return []*uint256.Int{w.UnitID(), txutil.SameShardID(w.UnitID(), w.HashForIdCalculation(hashFunc))}
}

func (w *swapWrapper) OwnerCondition() []byte      { return w.swap.OwnerCondition }
func (w *swapWrapper) Proofs() []*block.BlockProof { return w.swap.Proofs }
func (w *swapWrapper) TargetValue() uint64         { return w.swap.TargetValue }
func (w *swapWrapper) DCTransfers() []*transferDCWrapper {
	var sdt []*transferDCWrapper
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
func (w *swapWrapper) TargetUnits(_ crypto.Hash) []*uint256.Int {
	return []*uint256.Int{w.UnitID()}
}

func (w *wrapper) UnitID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(w.transaction.UnitId)
}

func (w *wrapper) Timeout() uint64 {
	return w.transaction.Timeout()
}

func (w *wrapper) SystemID() []byte {
	return w.transaction.SystemId
}

func (w *wrapper) OwnerProof() []byte {
	return w.transaction.OwnerProof
}

func (w *wrapper) ToProtoBuf() *txsystem.Transaction {
	return w.transaction
}

func (w *wrapper) IsPrimary() bool {
	return true
}

func (w *wrapper) SetServerMetadata(sm *txsystem.ServerMetadata) {
	w.ToProtoBuf().ServerMetadata = sm
	w.resetHasher()
}

func (w *wrapper) resetHasher() {
	w.hashValue = nil
}

func (w *wrapper) sigBytes(b *bytes.Buffer) {
	b.Write(w.transaction.SystemId)
	b.Write(w.transaction.UnitId)
	if w.transaction.ClientMetadata != nil {
		b.Write(w.transaction.ClientMetadata.Bytes())
	}
}

func (w *wrapper) addTransactionFieldsToHasher(hasher hash.Hash) {
	hasher.Write(w.transaction.SystemId)
	hasher.Write(w.transaction.UnitId)
	hasher.Write(w.transaction.OwnerProof)
	hasher.Write(w.transaction.FeeProof)
	if w.transaction.ClientMetadata != nil {
		hasher.Write(w.transaction.ClientMetadata.Bytes())
	}
	if w.transaction.ServerMetadata != nil {
		hasher.Write(w.transaction.ServerMetadata.Bytes())
	}
}

func (w *wrapper) hashComputed(hashFunc crypto.Hash) bool {
	return w.hashFunc == hashFunc && w.hashValue != nil
}
