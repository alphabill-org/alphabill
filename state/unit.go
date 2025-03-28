package state

import (
	"bytes"
	"crypto"
	"fmt"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
)

var _ Unit = (*UnitV1)(nil)

// UnitV1 is a node in the state tree. It is used to build state tree and unit ledgers.
type UnitV1 struct {
	logs                []*Log         // state changes of the unit during the current round
	logsHash            []byte         // root value of the hash tree built on the logs
	data                types.UnitData // current data of the unit
	deletionRound       uint64         // if deletionRound != 0 then the unit has been logically deleted, but the corresponding node will be actually deleted during the initialization of the deletionRound
	stateLockTx         []byte         // bytes of transaction that locked the unit
	subTreeSummaryValue uint64         // current summary value of the subtree rooted at this node
	subTreeSummaryHash  []byte         // summary hash of the subtree rooted at this node
	summaryCalculated   bool
}

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
		deletionRound:       u.deletionRound,
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
	return fmt.Sprintf("summaryCalculated=%v, nodeSummary=%d, subtreeSummary=%d",
		u.summaryCalculated, u.data.SummaryValueInput(), u.subTreeSummaryValue)
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

func (u *UnitV1) IsDummy() bool {
	return u.data == nil
}

func (u *UnitV1) AddUnitLog(hashAlgorithm crypto.Hash, txrHash []byte) error {
	unitLog, err := u.newUnitLog(hashAlgorithm, txrHash)
	if err != nil {
		return fmt.Errorf("failed to create unit log: %v", err)
	}
	u.logs = append(u.logs, unitLog)
	return nil
}

func (u *UnitV1) DeletionRound() uint64 {
	return u.deletionRound
}

func (u *UnitV1) IsExpired(currentRoundNumber uint64) bool {
	return u.deletionRound > 0 && u.deletionRound <= currentRoundNumber
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

func (u *UnitV1) latestUnitLog() *Log {
	l := len(u.logs)
	if l == 0 {
		return nil
	}
	return u.logs[l-1]
}

func (u *UnitV1) newUnitLog(hashAlgorithm crypto.Hash, txrHash []byte) (*Log, error) {
	unitLedgerHeadHash, err := u.unitLedgerHeadHash(hashAlgorithm, txrHash)
	if err != nil {
		return nil, fmt.Errorf("unable to hash unit ledger head: %w", err)
	}
	return NewUnitLog(txrHash, unitLedgerHeadHash, u.data, u.deletionRound, u.stateLockTx), nil
}

func (u *UnitV1) unitLedgerHeadHash(hashAlgorithm crypto.Hash, txrHash []byte) ([]byte, error) {
	if len(u.logs) == 0 {
		// if the unit is freshly created, initialize the unit ledger
		return abhash.HashValues(hashAlgorithm, nil, txrHash)
	}
	// if the unit existed earlier
	return abhash.HashValues(hashAlgorithm, u.logs[len(u.logs)-1].UnitLedgerHeadHash, txrHash)
}
