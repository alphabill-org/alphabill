package testpartition

import (
	"encoding/binary"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
)

type CounterTxSystem struct {
	ExecuteCount uint64
	SummaryValue uint64
	ExecuteDelta uint64
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

func (m *CounterTxSystem) State() txsystem.State {
	bytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(bytes, m.ExecuteCount)
	return &Summary{root: bytes, summary: util.Uint64ToBytes(m.SummaryValue)}
}

func (m *CounterTxSystem) BeginBlock(uint64) {
}

func (m *CounterTxSystem) Revert() {
}

func (m *CounterTxSystem) EndBlock() txsystem.State {
	count := m.ExecuteCount + m.ExecuteDelta
	bytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(bytes, count)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}
}

func (m *CounterTxSystem) Commit() {
	m.ExecuteCount += m.ExecuteDelta
	m.ExecuteDelta = 0
}

func (m *CounterTxSystem) Execute(_ *transaction.Transaction) error {
	m.ExecuteDelta++
	return nil
}
