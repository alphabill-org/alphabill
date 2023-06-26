package verifiable_data

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"hash"

	hasherUtil "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

var (
	ErrOwnerProofPresent = errors.New("'register data' transaction cannot have an owner proof")
	log                  = logger.CreateForPackage()
	txType               = "reg"
)

type (
	txSystem struct {
		systemIdentifier   []byte
		stateTree          *state.State
		hashAlgorithm      crypto.Hash
		currentBlockNumber uint64
	}

	unit struct {
		dataHash    []byte
		blockNumber uint64
	}
)

func New(systemId []byte) (*txSystem, error) {
	s := state.NewEmptyState()
	vdTxSystem := &txSystem{
		systemIdentifier: systemId,
		stateTree:        s,
		hashAlgorithm:    crypto.SHA256,
	}

	return vdTxSystem, nil
}

func (d *txSystem) StateSummary() (txsystem.State, error) {
	if !d.stateTree.IsCommitted() {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	return d.getState(), nil
}

func (d *txSystem) BeginBlock(blockNumber uint64) {
	d.currentBlockNumber = blockNumber
}
func (d *txSystem) ValidatorGeneratedTransactions() ([]*types.TransactionRecord, error) {
	return nil, nil
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
	sm := &types.ServerMetadata{ActualFee: 0, TargetUnits: []types.UnitID{tx.UnitID()}}
	if err := d.stateTree.Apply(
		state.AddUnit(tx.UnitID(),
			script.PredicateAlwaysFalse(),
			&unit{
				dataHash:    hasherUtil.Sum256(tx.UnitID()),
				blockNumber: d.currentBlockNumber,
			},
		)); err != nil {
		return nil, fmt.Errorf("could not add item: %w", err)
	}
	return sm, nil
}

func (d *txSystem) getState() txsystem.State {
	sv, root, err := d.stateTree.CalculateRoot()
	if err != nil {
		return nil
	}
	if root == nil {
		return txsystem.NewStateSummary(make([]byte, 32), util.Uint64ToBytes(sv))
	}
	return txsystem.NewStateSummary(root, util.Uint64ToBytes(sv))
}

func (u *unit) Write(hasher hash.Hash) {
	hasher.Write(u.dataHash)
	hasher.Write(util.Uint64ToBytes(u.blockNumber))
}

func (u *unit) SummaryValueInput() uint64 {
	return 0
}

func (u *unit) Copy() state.UnitData {
	return &unit{
		dataHash:    bytes.Clone(u.dataHash),
		blockNumber: u.blockNumber,
	}
}
