package testtxsystem

import (
	"encoding/binary"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	log "github.com/alphabill-org/alphabill/pkg/logger"
)

var logger = log.CreateForPackage()

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
	logger.Debug("CounterTxSystem: Root(): %X", s.root)
	return s.root
}

func (s *Summary) Summary() []byte {
	logger.Debug("CounterTxSystem: Summary(): %X", s.summary)
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
	logger.Debug("CounterTxSystem: State(%d): %X", m.blockNo, bytes)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}, nil
}

func (m *CounterTxSystem) BeginBlock(nr uint64) {
	logger.Debug("CounterTxSystem: BeginBlock(%d)", nr)
	m.BeginBlockCountDelta++
	m.ExecuteCountDelta = 0
	m.blockNo = nr
	m.uncommitted = true
}

func (m *CounterTxSystem) Revert() {
	logger.Debug("CounterTxSystem: Revert(%d)", m.blockNo)
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
		state += m.EndBlockCount + m.EndBlockCountDelta
	}
	binary.LittleEndian.PutUint64(bytes, state)
	logger.Debug("CounterTxSystem: EndBlock(%d), resulting state: %X", m.blockNo, bytes)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}, nil
}

func (m *CounterTxSystem) Commit() error {
	m.ExecuteCount += m.ExecuteCountDelta
	m.EndBlockCount += m.EndBlockCountDelta
	m.BeginBlockCount += m.BeginBlockCountDelta
	m.uncommitted = false
	summary, _ := m.StateSummary()
	logger.Debug("CounterTxSystem: Commit(%d), state: %X", m.blockNo, summary.Root())
	return nil
}

func (m *CounterTxSystem) Execute(_ *types.TransactionOrder) (*types.ServerMetadata, error) {
	logger.Debug("CounterTxSystem: Execute()")
	m.ExecuteCountDelta++
	return &types.ServerMetadata{ActualFee: m.Fee}, nil
}
