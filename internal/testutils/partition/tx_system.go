package partition

import (
	"encoding/binary"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
)

type (
	MockTxSystem struct {
		BeginBlockCount uint64
		EndBlockCount   uint64
		ExecuteCount    uint64
		RevertCount     uint64
		SummaryValue    uint64
	}
)

func (m *MockTxSystem) BeginBlock() {
	m.BeginBlockCount++
}

func (m *MockTxSystem) Revert() {
	m.RevertCount++
}

func (m *MockTxSystem) EndBlock() ([]byte, state.SummaryValue) {
	m.EndBlockCount++
	bytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(bytes, m.EndBlockCount)
	return bytes, state.Uint64SummaryValue(m.SummaryValue)
}

func (m *MockTxSystem) Execute(_ *transaction.Transaction) error {
	m.ExecuteCount++
	return nil
}
