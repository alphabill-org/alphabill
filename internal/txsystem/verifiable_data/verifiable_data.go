package verifiable_data

import (
	"crypto"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	hasherUtil "gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const zeroSummaryValue = state.Uint64SummaryValue(0)

var (
	ErrOwnerProofPresent = errors.New("'register data' transaction cannot have an owner proof")

	zeroRootHash = make([]byte, 32)
)

type (
	stateTree interface {
		AddItem(id *uint256.Int, owner state.Predicate, data state.UnitData, stateHash []byte) error
		GetRootHash() []byte
		Commit()
		Revert()
	}

	txSystem struct {
		stateTree          stateTree
		hashAlgorithm      crypto.Hash
		currentBlockNumber uint64
	}

	vdState struct {
		root    []byte
		summary []byte
	}

	unit struct {
		dataHash    []byte
		blockNumber uint64
	}
)

func (s vdState) Root() []byte {
	return s.root
}

func (s vdState) Summary() []byte {
	return s.summary
}

func New() (*txSystem, error) {
	conf := &state.Config{HashAlgorithm: crypto.SHA256}
	s, err := state.New(conf)
	if err != nil {
		return nil, err
	}

	vdTxSystem := &txSystem{
		stateTree:     s,
		hashAlgorithm: conf.HashAlgorithm,
	}

	return vdTxSystem, nil
}

func (d *txSystem) State() txsystem.State {
	if d.stateTree.GetRootHash() == nil {
		return &vdState{zeroRootHash, zeroSummaryValue.Bytes()}
	}
	return &vdState{d.stateTree.GetRootHash(), zeroSummaryValue.Bytes()}
}

func (d *txSystem) BeginBlock(blockNumber uint64) {
	d.currentBlockNumber = blockNumber
}

func (d *txSystem) EndBlock() txsystem.State {
	return d.State()
}

func (d *txSystem) Revert() {
	d.stateTree.Revert()
}

func (d *txSystem) Commit() {
	d.stateTree.Commit()
}

func (d *txSystem) Execute(tx *transaction.Transaction) error {
	if len(tx.OwnerProof) > 0 {
		return ErrOwnerProofPresent
	}
	h, err := tx.Hash(d.hashAlgorithm)
	if err != nil {
		return errors.Wrapf(err, "failed to hash tx: %v", err)
	}
	err = d.stateTree.AddItem(
		uint256.NewInt(0).SetBytes(tx.UnitId),
		script.PredicateAlwaysFalse(),
		&unit{
			dataHash:    hasherUtil.Sum256(tx.UnitId),
			blockNumber: d.currentBlockNumber,
		},
		h,
	)
	if err != nil {
		return errors.Wrapf(err, "could not add item: %v", err)
	}
	return nil
}

func (u *unit) AddToHasher(hasher hash.Hash) {
	hasher.Write(u.dataHash)
	hasher.Write(util.Uint64ToBytes(u.blockNumber))
}

func (u *unit) Value() state.SummaryValue {
	return zeroSummaryValue
}
