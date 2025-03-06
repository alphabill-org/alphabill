package state

import (
	"bytes"
	"crypto"
	"fmt"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
)

var _ Unit = (*UnitV1)(nil)

type (
	// UnitV1 is a node in the state tree. It is used to build state tree and unit ledgers.
	UnitV1 struct {
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

func NewUnit(data types.UnitData) *UnitV1 {
	return &UnitV1{
		data: data,
	}
}

func (u *UnitV1) GetVersion() types.ABVersion {
	// no need for the version field
	// because this struct is not serialized directly
	return 1
}

func (u *UnitV1) Clone() Unit {
	if u == nil {
		return nil
	}
	return &UnitV1{
		logs:                copyLogs(u.logs),
		stateLockTx:         bytes.Clone(u.stateLockTx),
		data:                copyData(u.data),
		subTreeSummaryValue: u.subTreeSummaryValue,
		summaryCalculated:   false,
	}
}

func ToUnitV1(u Unit) (*UnitV1, error) {
	if u == nil {
		return nil, fmt.Errorf("unit is nil")
	}
	if u.GetVersion() != 1 {
		return nil, fmt.Errorf("unexpected unit version: %d, need 1", u.GetVersion())
	}
	return u.(*UnitV1), nil
}

func (u *UnitV1) String() string {
	return fmt.Sprintf("summaryCalculated=%v, nodeSummary=%d, subtreeSummary=%d", u.summaryCalculated, u.data.SummaryValueInput(), u.subTreeSummaryValue)
}

func (u *UnitV1) IsStateLocked() bool {
	return len(u.stateLockTx) > 0
}

func (u *UnitV1) StateLockTx() []byte {
	return bytes.Clone(u.stateLockTx)
}

func (u *UnitV1) Data() types.UnitData {
	return copyData(u.data)
}

func (u *UnitV1) Logs() []*Log {
	return u.logs
}

func (u *UnitV1) LastLogIndex() int {
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

func (l *Log) Hash(algorithm crypto.Hash) ([]byte, error) {
	hasher := abhash.New(algorithm.New())
	if l.NewUnitData != nil {
		l.NewUnitData.Write(hasher)
	}
	// @todo: AB-1609 add delete round "nd"
	if l.NewStateLockTx != nil {
		hasher.Write(l.NewStateLockTx)
	}
	//y_j
	dataHash, err := hasher.Sum()
	if err != nil {
		return nil, fmt.Errorf("failed to hash log data: %w", err)
	}
	hasher.Reset()
	hasher.Write(l.UnitLedgerHeadHash)
	hasher.Write(dataHash)
	// z_j
	return hasher.Sum()
}

func (u *UnitV1) latestUnitData() types.UnitData {
	l := len(u.logs)
	if l == 0 {
		return u.data
	}
	return u.logs[l-1].NewUnitData
}

func (u *UnitV1) latestStateLockTx() []byte {
	l := len(u.logs)
	if l == 0 {
		return u.stateLockTx
	}
	return u.logs[l-1].NewStateLockTx
}
