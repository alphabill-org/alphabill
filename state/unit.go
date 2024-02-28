package state

import (
	"bytes"
	"crypto"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

type (
	// Unit is a node in the state tree. It is used to build state tree and unit ledgers.
	Unit struct {
		logs                []*Log               // state changes of the unit during the current round
		logsHash            []byte               // root value of the hash tree built on the logs
		bearer              types.PredicateBytes // current bearer condition
		data                UnitData             // current data of the unit
		subTreeSummaryValue uint64               // current summary value of the sub-tree rooted at this node
		subTreeSummaryHash  []byte               // summary hash of the sub-tree rooted at this node
		summaryCalculated   bool
	}

	// UnitData is a generic data type for the unit state.
	UnitData interface {
		Write(hasher hash.Hash) error
		SummaryValueInput() uint64
		Copy() UnitData
	}

	// Log contains a state changes of the unit during the transaction execution.
	Log struct {
		TxRecordHash       []byte // the hash of the transaction record that brought the unit to the state described by given log entry.
		UnitLedgerHeadHash []byte // the new head hash of the unit ledger
		NewBearer          types.PredicateBytes
		NewUnitData        UnitData
	}
)

func NewUnit(bearer types.PredicateBytes, data UnitData) *Unit {
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
		bearer:              bytes.Clone(u.bearer),
		data:                copyData(u.data),
		subTreeSummaryValue: u.subTreeSummaryValue,
		summaryCalculated:   false,
	}
}

func (u *Unit) String() string {
	return fmt.Sprintf("summaryCalculated=%v, nodeSummary=%d, subtreeSummary=%d", u.summaryCalculated, u.data.SummaryValueInput(), u.subTreeSummaryValue)
}

func (u *Unit) Bearer() types.PredicateBytes {
	return bytes.Clone(u.bearer)
}

func (u *Unit) Data() UnitData {
	return copyData(u.data)
}

func (u *Unit) Logs() []*Log {
	return u.logs
}

func (u *Unit) LastLogIndex() int {
	return len(u.logs) - 1
}

func MarshalUnitData(u UnitData) ([]byte, error) {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, fmt.Errorf("cbor encoder init failed: %w", err)
	}
	return enc.Marshal(u)
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
		NewBearer:          bytes.Clone(l.NewBearer),
		NewUnitData:        copyData(l.NewUnitData),
	}
}

func (l *Log) Hash(algorithm crypto.Hash) []byte {
	hasher := algorithm.New()
	hasher.Write(l.NewBearer)
	if l.NewUnitData != nil {
		// todo: change Hash interface to allow errors
		_ = l.NewUnitData.Write(hasher)
	}
	//y_j
	dataHash := hasher.Sum(nil)
	hasher.Reset()
	hasher.Write(l.UnitLedgerHeadHash)
	hasher.Write(dataHash)
	// z_j
	return hasher.Sum(nil)
}

func (u *Unit) latestUnitBearer() []byte {
	l := len(u.logs)
	if l == 0 {
		return u.bearer
	}
	return u.logs[l-1].NewBearer
}

func (u *Unit) latestUnitData() UnitData {
	l := len(u.logs)
	if l == 0 {
		return u.data
	}
	return u.logs[l-1].NewUnitData
}
