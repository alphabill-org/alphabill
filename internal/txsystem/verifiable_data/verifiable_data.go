package verifiable_data

import (
	"crypto"
	"errors"
	"fmt"
	"hash"

	hasherUtil "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

const zeroSummaryValue = rma.Uint64SummaryValue(0)

var (
	ErrOwnerProofPresent = errors.New("'register data' transaction cannot have an owner proof")
	log                  = logger.CreateForPackage()
	zeroRootHash         = make([]byte, 32)

	txType = "reg"
)

type (
	/*
		vdTransaction struct {
			transaction *txsystem.Transaction
			hashFunc    crypto.Hash
			hashValue   []byte
		}*/

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

func (d *txSystem) StateSummary() (txsystem.State, error) {
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

func (d *txSystem) Execute(tx *types.TransactionOrder) (*types.ServerMetadata, error) {
	log.Debug("Processing register data tx: '%v', UnitID=%x", tx, tx.UnitID())
	if len(tx.OwnerProof) > 0 {
		return nil, ErrOwnerProofPresent
	}
	if tx.PayloadType() != txType {
		return nil, fmt.Errorf("invalid transaction payload type: got %s, expected %s", tx.PayloadType(), txType)
	}
	h := tx.Hash(d.hashAlgorithm)
	if err := d.stateTree.AtomicUpdate(
		rma.AddItem(util.BytesToUint256(tx.UnitID()),
			script.PredicateAlwaysFalse(),
			&unit{
				dataHash:    hasherUtil.Sum256(tx.UnitID()),
				blockNumber: d.currentBlockNumber,
			},
			h,
		)); err != nil {
		return nil, fmt.Errorf("could not add item: %v", err)
	}
	return &types.ServerMetadata{}, nil
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

/*
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


*/
