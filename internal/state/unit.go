package state

import (
	"bytes"
	"crypto"
	"fmt"
	"hash"
)

type (

	// Unit is a node in the state tree. It is used to build state tree and unit ledgers.
	Unit struct {
		logs                logs      // state changes of the unit.
		logRoot             []byte    // root value of the hash tree built on the state log.
		bearer              Predicate // current bearer condition
		data                UnitData  // current data of the unit
		subTreeSummaryValue uint64    // current summary value of the sub-tree rooted at this node
		subTreeSummaryHash  []byte    // summary hash of the sub-tree rooted at this node,
	}

	// UnitData is a generic data type for the unit state.
	UnitData interface {
		Write(hasher hash.Hash)
		SummaryValueInput() uint64
		Copy() UnitData
	}

	// log contains a state changes of the unit during the transaction execution.
	log struct {
		txRecordHash       []byte // the hash of the transaction record that brought the unit to the state described by given log entry.
		unitLedgerHeadHash []byte // the new head hash of the unit ledger
		newBearer          Predicate
		newUnitData        UnitData
	}

	// logs contains a state changes of the unit during the current round.
	logs []*log

	Predicate []byte
)

func NewUnit(bearer Predicate, data UnitData) *Unit {
	return &Unit{
		bearer: bearer,
		data:   data,
	}
}

func (u *Unit) Clone() *Unit {
	if u == nil {
		return nil
	}
	return &Unit{
		logs:                copyLogs(u.logs),
		logRoot:             bytes.Clone(u.logRoot),
		bearer:              bytes.Clone(u.bearer),
		data:                copyData(u.data),
		subTreeSummaryValue: u.subTreeSummaryValue,
	}
}

func (u *Unit) String() string {
	return fmt.Sprintf("nodeSummary=%d, subtreeSummary=%d", u.data.SummaryValueInput(), u.subTreeSummaryValue)
}

func (u *Unit) Bearer() Predicate {
	return bytes.Clone(u.bearer)
}

func (u *Unit) Data() UnitData {
	return copyData(u.data)
}

func copyLogs(entries logs) logs {
	logsCopy := make([]*log, len(entries))
	for i, e := range entries {
		logsCopy[i] = e.Clone()
	}
	return logsCopy
}

func (l *log) Clone() *log {
	if l == nil {
		return nil
	}
	return &log{
		txRecordHash:       bytes.Clone(l.txRecordHash),
		unitLedgerHeadHash: bytes.Clone(l.unitLedgerHeadHash),
		newBearer:          bytes.Clone(l.newBearer),
		newUnitData:        copyData(l.newUnitData),
	}
}

func (l *log) Hash(algorithm crypto.Hash) []byte {
	hasher := algorithm.New()
	hasher.Write(l.newBearer)
	if l.newUnitData != nil {
		l.newUnitData.Write(hasher)
	}
	//y_j
	dataHash := hasher.Sum(nil)
	hasher.Reset()
	hasher.Write(l.unitLedgerHeadHash)
	hasher.Write(dataHash)
	// z_j
	return hasher.Sum(nil)
}

func (u *Unit) latestUnitBearer() []byte {
	l := len(u.logs)
	if l == 0 {
		return u.bearer
	}
	return u.logs[l-1].newBearer
}

func (u *Unit) latestUnitData() UnitData {
	l := len(u.logs)
	if l == 0 {
		return u.data
	}
	return u.logs[l-1].newUnitData
}
