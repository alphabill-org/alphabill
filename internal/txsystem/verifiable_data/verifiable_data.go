package verifiable_data

import (
	"bytes"
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/errors"
	hasherUtil "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const zeroSummaryValue = rma.Uint64SummaryValue(0)

var (
	ErrOwnerProofPresent = errors.New("'register data' transaction cannot have an owner proof")
	log                  = logger.CreateForPackage()
	zeroRootHash         = make([]byte, 32)
)

type (
	vdTransaction struct {
		transaction *txsystem.Transaction
		hashFunc    crypto.Hash
		hashValue   []byte
	}

	txSystem struct {
		systemIdentifier   []byte
		stateTree          *rma.Tree
		hashAlgorithm      crypto.Hash
		currentBlockNumber uint64
	}

	unit struct {
		dataHash    []byte
		blockNumber uint64
	}
)

func New(systemId []byte) (*txSystem, error) {
	conf := &rma.Config{HashAlgorithm: crypto.SHA256}
	s, err := rma.New(conf)
	if err != nil {
		return nil, err
	}

	vdTxSystem := &txSystem{
		systemIdentifier: systemId,
		stateTree:        s,
		hashAlgorithm:    conf.HashAlgorithm,
	}

	return vdTxSystem, nil
}

func (d *txSystem) State() (txsystem.State, error) {
	if d.stateTree.ContainsUncommittedChanges() {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	return d.getState(), nil
}

func (d *txSystem) BeginBlock(blockNumber uint64) {
	d.currentBlockNumber = blockNumber
}

func (d *txSystem) EndBlock() (txsystem.State, error) {
	return d.getState(), nil
}

func (d *txSystem) Revert() {
	d.stateTree.Revert()
}

func (d *txSystem) Commit() {
	d.stateTree.Commit()
}

func (d *txSystem) Execute(tx txsystem.GenericTransaction) error {
	log.Debug("Processing register data tx: '%v', UnitID=%x", tx, tx.UnitID())
	if len(tx.OwnerProof()) > 0 {
		return ErrOwnerProofPresent
	}
	h := tx.Hash(d.hashAlgorithm)
	err := d.stateTree.AtomicUpdate(
		rma.AddItem(tx.UnitID(),
			script.PredicateAlwaysFalse(),
			&unit{
				dataHash:    hasherUtil.Sum256(util.Uint256ToBytes(tx.UnitID())),
				blockNumber: d.currentBlockNumber,
			},
			h,
		))
	if err != nil {
		return errors.Wrapf(err, "could not add item: %v", err)
	}
	return nil
}

func (d *txSystem) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	if !bytes.Equal(d.systemIdentifier, tx.GetSystemId()) {
		return nil, errors.Errorf("transaction has invalid system identifier %X, expected %X", tx.GetSystemId(), d.systemIdentifier)
	}
	if tx.TransactionAttributes != nil {
		return nil, errors.New("invalid vd transaction: transactionAttributes present")
	}

	return &vdTransaction{
		transaction: tx,
	}, nil
}

func (d *txSystem) getState() txsystem.State {
	if d.stateTree.GetRootHash() == nil {
		return txsystem.NewStateSummary(zeroRootHash, zeroSummaryValue.Bytes())
	}
	return txsystem.NewStateSummary(d.stateTree.GetRootHash(), zeroSummaryValue.Bytes())
}

func (u *unit) AddToHasher(hasher hash.Hash) {
	hasher.Write(u.dataHash)
	hasher.Write(util.Uint64ToBytes(u.blockNumber))
}

func (u *unit) Value() rma.SummaryValue {
	return zeroSummaryValue
}

func (w *vdTransaction) Hash(hashFunc crypto.Hash) []byte {
	if w.hashComputed(hashFunc) {
		return w.hashValue
	}
	hasher := hashFunc.New()
	w.AddToHasher(hasher)

	w.hashValue = hasher.Sum(nil)
	w.hashFunc = hashFunc
	return w.hashValue
}

func (w *vdTransaction) AddToHasher(hasher hash.Hash) {
	hasher.Write(w.transaction.Bytes())
}

func (w *vdTransaction) SigBytes() []byte {
	return nil
}

func (w *vdTransaction) UnitID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(w.transaction.UnitId)
}

func (w *vdTransaction) Timeout() uint64 {
	return w.transaction.Timeout()
}

func (w *vdTransaction) SystemID() []byte {
	return w.transaction.SystemId
}

func (w *vdTransaction) OwnerProof() []byte {
	return w.transaction.OwnerProof
}

func (w *vdTransaction) ToProtoBuf() *txsystem.Transaction {
	return w.transaction
}

func (w *vdTransaction) IsPrimary() bool {
	return true
}

func (w *vdTransaction) TargetUnits(_ crypto.Hash) []*uint256.Int {
	return []*uint256.Int{w.UnitID()}
}

func (w *vdTransaction) SetServerMetadata(sm *txsystem.ServerMetadata) {
	w.ToProtoBuf().ServerMetadata = sm
	w.resetHasher()
}

func (w *vdTransaction) resetHasher() {
	w.hashValue = nil
}

func (w *vdTransaction) sigBytes(b *bytes.Buffer) {
	b.Write(w.transaction.SystemId)
	b.Write(w.transaction.UnitId)
	if w.transaction.ClientMetadata != nil {
		b.Write(w.transaction.ClientMetadata.Bytes())
	}
}

func (w *vdTransaction) hashComputed(hashFunc crypto.Hash) bool {
	return w.hashFunc == hashFunc && w.hashValue != nil
}
