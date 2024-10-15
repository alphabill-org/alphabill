package state

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	// Unit is a node in the state tree. It is used to build state tree and unit ledgers.
	Unit struct {
		logs                []*Log         // state changes of the unit during the current round
		logsHash            []byte         // root value of the hash tree built on the logs
		data                types.UnitData // current data of the unit
		stateLockTx         []byte         // bytes of transaction that locked the unit
		subTreeSummaryValue uint64         // current summary value of the sub-tree rooted at this node
		subTreeSummaryHash  []byte         // summary hash of the sub-tree rooted at this node
		summaryCalculated   bool
	}

	// Log contains a state changes of the unit during the transaction execution.
	Log struct {
		TxRecordHash       []byte // the hash of the transaction record that brought the unit to the state described by given log entry.
		UnitLedgerHeadHash []byte // the new head hash of the unit ledger
		NewUnitData        types.UnitData
		NewStateLockTx     []byte
	}
)

func NewUnit(data types.UnitData) *Unit {
	return &Unit{
		data: data,
	}
}

func (u *Unit) Clone() *Unit {
	if u == nil {
		return nil
	}
	return &Unit{
		logs:                copyLogs(u.logs),
		stateLockTx:         bytes.Clone(u.stateLockTx),
		data:                copyData(u.data),
		subTreeSummaryValue: u.subTreeSummaryValue,
		summaryCalculated:   false,
	}
}

func (u *Unit) String() string {
	return fmt.Sprintf("summaryCalculated=%v, nodeSummary=%d, subtreeSummary=%d", u.summaryCalculated, u.data.SummaryValueInput(), u.subTreeSummaryValue)
}

func (u *Unit) IsStateLocked() bool {
	return len(u.stateLockTx) > 0
}

func (u *Unit) StateLockTx() []byte {
	return bytes.Clone(u.stateLockTx)
}

func (u *Unit) Data() types.UnitData {
	return copyData(u.data)
}

func (u *Unit) Logs() []*Log {
	return u.logs
}

func (u *Unit) LastLogIndex() int {
	return len(u.logs) - 1
}

func MarshalUnitData(u types.UnitData) ([]byte, error) {
	return types.Cbor.Marshal(u)
}

func copyLogs(entries []*Log) []*Log {
	logsCopy := make([]*Log, len(entries))
	for i, e := range entries {
		logsCopy[i] = e.Clone()
	}
	return logsCopy
}

func (l *Log) Clone() *Log {
	if l == nil {
		return nil
	}
	return &Log{
		TxRecordHash:       bytes.Clone(l.TxRecordHash),
		UnitLedgerHeadHash: bytes.Clone(l.UnitLedgerHeadHash),
		NewUnitData:        copyData(l.NewUnitData),
		NewStateLockTx:     bytes.Clone(l.NewStateLockTx),
	}
}

func (l *Log) Hash(algorithm crypto.Hash) []byte {
	hasher := algorithm.New()
	if l.NewUnitData != nil {
		// todo: change Hash interface to allow errors
		_ = l.NewUnitData.Write(hasher)
	}
	// @todo: AB-1609 add delete round "nd"
	if l.NewStateLockTx != nil {
		hasher.Write(l.NewStateLockTx)
	}
	//y_j
	dataHash := hasher.Sum(nil)
	hasher.Reset()
	hasher.Write(l.UnitLedgerHeadHash)
	hasher.Write(dataHash)
	// z_j
	return hasher.Sum(nil)
}

func (u *Unit) latestUnitData() types.UnitData {
	l := len(u.logs)
	if l == 0 {
		return u.data
	}
	return u.logs[l-1].NewUnitData
}

func (u *Unit) latestStateLockTx() []byte {
	l := len(u.logs)
	if l == 0 {
		return u.stateLockTx
	}
	return u.logs[l-1].NewStateLockTx
}
