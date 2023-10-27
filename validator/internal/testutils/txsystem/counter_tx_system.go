package testtxsystem

import (
	"encoding/binary"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/common/util"
	"github.com/alphabill-org/alphabill/txsystem"
)

type CounterTxSystem struct {
	InitCount       uint64
	BeginBlockCount uint64
	EndBlockCount   uint64
	ExecuteCount    uint64
	RevertCount     uint64
	SummaryValue    uint64

	ExecuteCountDelta    uint64
	EndBlockCountDelta   uint64
	BeginBlockCountDelta uint64

	// setting this affects the state once EndBlock() is called
	EndBlockChangesState bool

	blockNo     uint64
	uncommitted bool

	// fee charged for each tx
	Fee uint64
}

type Summary struct {
	root    []byte
	summary []byte
}

func (s *Summary) Root() []byte {
	return s.root
}

func (s *Summary) Summary() []byte {
	return s.summary
}

func (m *CounterTxSystem) StateSummary() (txsystem.State, error) {
	if m.uncommitted {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	bytes := make([]byte, 32)
	var state = m.InitCount + m.ExecuteCount
	if m.EndBlockChangesState {
		state += m.EndBlockCount
	}
	binary.LittleEndian.PutUint64(bytes, state)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}, nil
}

func (m *CounterTxSystem) BeginBlock(nr uint64) error {
	m.blockNo = nr
	m.BeginBlockCountDelta++
	m.ExecuteCountDelta = 0
	return nil
}

func (m *CounterTxSystem) Revert() {
	m.ExecuteCountDelta = 0
	m.EndBlockCountDelta = 0
	m.BeginBlockCountDelta = 0
	m.RevertCount++
	m.uncommitted = false
}

func (m *CounterTxSystem) EndBlock() (txsystem.State, error) {
	m.EndBlockCountDelta++
	bytes := make([]byte, 32)
	var state = m.InitCount + m.ExecuteCount + m.ExecuteCountDelta
	if m.EndBlockChangesState {
		m.uncommitted = true
		state += m.EndBlockCount + m.EndBlockCountDelta
	}
	binary.LittleEndian.PutUint64(bytes, state)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}, nil
}

func (m *CounterTxSystem) Commit() error {
	m.ExecuteCount += m.ExecuteCountDelta
	m.EndBlockCount += m.EndBlockCountDelta
	m.BeginBlockCount += m.BeginBlockCountDelta
	m.uncommitted = false
	return nil
}

func (m *CounterTxSystem) Execute(_ *types.TransactionOrder) (*types.ServerMetadata, error) {
	m.ExecuteCountDelta++
	m.uncommitted = true
	return &types.ServerMetadata{ActualFee: m.Fee}, nil
}
