package testtxsystem

import (
	"encoding/binary"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
)

type CounterTxSystem struct {
	InitCount       uint64
	BeginBlockCount uint64
	EndBlockCount   uint64
	ExecuteCount    uint64
	RevertCount     uint64
	SummaryValue    uint64

	EndBlockCountDelta   uint64
	BeginBlockCountDelta uint64
}

func (m *CounterTxSystem) Init() ([]byte, state.SummaryValue) {
	bytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(bytes, m.InitCount)
	return bytes, state.Uint64SummaryValue(m.SummaryValue)
}

func (m *CounterTxSystem) BeginBlock() {
	m.BeginBlockCountDelta++
}

func (m *CounterTxSystem) Revert() {
	m.EndBlockCountDelta = 0
	m.BeginBlockCountDelta = 0
	m.RevertCount++
}

func (m *CounterTxSystem) EndBlock() ([]byte, state.SummaryValue) {
	m.EndBlockCountDelta++
	bytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(bytes, m.EndBlockCount+m.EndBlockCountDelta)
	return bytes, state.Uint64SummaryValue(m.SummaryValue)
}

func (m *CounterTxSystem) Commit() {
	m.EndBlockCount += m.EndBlockCountDelta
	m.BeginBlockCount += m.BeginBlockCountDelta
}

func (m *CounterTxSystem) Execute(_ *transaction.Transaction) error {
	m.ExecuteCount++
	return nil
}
