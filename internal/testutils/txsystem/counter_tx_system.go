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
}

type Summary struct {
	root    []byte
	summary []byte
}

func (s *Summary) Root() []byte {
	logger.Debug("CounterTxSystem: Root()")
	return s.root
}

func (s *Summary) Summary() []byte {
	logger.Debug("CounterTxSystem: Summary()")
	return s.summary
}

func (m *CounterTxSystem) StateSummary() (txsystem.State, error) {
	logger.Debug("CounterTxSystem: State()")
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

func (m *CounterTxSystem) BeginBlock(_ uint64) {
	logger.Debug("CounterTxSystem: BeginBlock()")
	m.BeginBlockCountDelta++
	m.ExecuteCountDelta = 0
}

func (m *CounterTxSystem) ValidatorGeneratedTransactions() ([]*types.TransactionRecord, error) {
	return nil, nil
}

func (m *CounterTxSystem) Revert() {
	logger.Debug("CounterTxSystem: Revert()")
	m.ExecuteCountDelta = 0
	m.EndBlockCountDelta = 0
	m.BeginBlockCountDelta = 0
	m.RevertCount++
}

func (m *CounterTxSystem) EndBlock() (txsystem.State, error) {
	logger.Debug("CounterTxSystem: EndBlock()")
	m.EndBlockCountDelta++
	bytes := make([]byte, 32)
	var state = m.InitCount + m.ExecuteCount + m.ExecuteCountDelta
	if m.EndBlockChangesState {
		state += m.EndBlockCount + m.EndBlockCountDelta
	}
	binary.LittleEndian.PutUint64(bytes, state)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}, nil
}

func (m *CounterTxSystem) Commit() error {
	logger.Debug("CounterTxSystem: Commit()")
	m.ExecuteCount += m.ExecuteCountDelta
	m.EndBlockCount += m.EndBlockCountDelta
	m.BeginBlockCount += m.BeginBlockCountDelta
	return nil
}

func (m *CounterTxSystem) Execute(_ *types.TransactionOrder) (*types.ServerMetadata, error) {
	logger.Debug("CounterTxSystem: Execute()")
	m.ExecuteCountDelta++
	return &types.ServerMetadata{}, nil
}
