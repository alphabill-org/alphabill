package testpartition

import (
	"encoding/binary"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
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

func (m *CounterTxSystem) State() (txsystem.State, error) {
	bytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(bytes, m.ExecuteCount)
	return &Summary{root: bytes, summary: util.Uint64ToBytes(m.SummaryValue)}, nil
}

func (m *CounterTxSystem) BeginBlock(_ uint64) {
}

func (m *CounterTxSystem) Revert() {
}

func (m *CounterTxSystem) EndBlock() (txsystem.State, error) {
	count := m.ExecuteCount + m.ExecuteDelta
	bytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(bytes, count)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}, nil
}

func (m *CounterTxSystem) Commit() {
	m.ExecuteCount += m.ExecuteDelta
	m.ExecuteDelta = 0
}

func (m *CounterTxSystem) Execute(_ txsystem.GenericTransaction) error {
	m.ExecuteDelta++
	return nil
}

func (m *CounterTxSystem) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return txsystem.NewDefaultGenericTransaction(tx)
}
