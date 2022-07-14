package testtxsystem

import (
	"encoding/binary"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
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
	binary.LittleEndian.PutUint64(bytes, m.InitCount)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}, nil
}

func (m *CounterTxSystem) BeginBlock(_ uint64) {
	m.BeginBlockCountDelta++
}

func (m *CounterTxSystem) Revert() {
	m.EndBlockCountDelta = 0
	m.BeginBlockCountDelta = 0
	m.RevertCount++
}

func (m *CounterTxSystem) EndBlock() (txsystem.State, error) {
	m.EndBlockCountDelta++
	bytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(bytes, m.EndBlockCount+m.EndBlockCountDelta)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}, nil
}

func (m *CounterTxSystem) Commit() {
	m.EndBlockCount += m.EndBlockCountDelta
	m.BeginBlockCount += m.BeginBlockCountDelta
}

func (m *CounterTxSystem) Execute(_ txsystem.GenericTransaction) error {
	m.ExecuteCount++
	return nil
}

func (m *CounterTxSystem) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return txsystem.NewDefaultGenericTransaction(tx)
}
