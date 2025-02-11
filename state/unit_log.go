package state

import (
	"bytes"
	"crypto"
	"fmt"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
)

// Log contains a state changes of the unit during the transaction execution.
type Log struct {
	TxRecordHash       []byte // the hash of the transaction record that brought the unit to the state described by given log entry.
	UnitLedgerHeadHash []byte // the new head hash of the unit ledger
	NewUnitData        types.UnitData
	DeletionRound      uint64
	NewStateLockTx     []byte
}

func NewUnitLog(txrHash []byte, unitLedgerHeadHash []byte, unitData types.UnitData, deletionRound uint64, stateLockTx []byte) *Log {
	return &Log{
		TxRecordHash:       bytes.Clone(txrHash),
		UnitLedgerHeadHash: bytes.Clone(unitLedgerHeadHash),
		NewUnitData:        copyData(unitData),
		DeletionRound:      deletionRound,
		NewStateLockTx:     bytes.Clone(stateLockTx),
	}
}

func (l *Log) Clone() *Log {
	if l == nil {
		return nil
	}
	return NewUnitLog(l.TxRecordHash, l.UnitLedgerHeadHash, l.NewUnitData, l.DeletionRound, l.NewStateLockTx)
}

func (l *Log) Hash(algorithm crypto.Hash) ([]byte, error) {
	// y_j
	unitState, err := types.NewUnitState(l.NewUnitData, l.DeletionRound, l.NewStateLockTx)
	if err != nil {
		return nil, fmt.Errorf("failed to create unit state data: %w", err)
	}
	unitStateHash, err := unitState.Hash(crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to hash unit state: %w", err)
	}

	// z_j
	hasher := abhash.New(algorithm.New())
	hasher.Write(l.UnitLedgerHeadHash)
	hasher.Write(unitStateHash)
	return hasher.Sum()
}

func (l *Log) UnitState() (*types.UnitState, error) {
	return types.NewUnitState(l.NewUnitData, l.DeletionRound, l.NewStateLockTx)
}
