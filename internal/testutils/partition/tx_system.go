package partition

import (
	"encoding/binary"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
)

type (
	MockTxSystem struct {
		RoundInitCount     uint64
		RoundCompleteCount uint64
		ExecuteCount       uint64
		RevertCount        uint64
		SummaryValue       uint64
	}

	Uint64SummaryValue uint64
)

func (m *MockTxSystem) RInit() {
	m.RoundInitCount++
}
func (m *MockTxSystem) Revert() {
	m.RevertCount++
}

func (m *MockTxSystem) RCompl() ([]byte, state.SummaryValue) {
	m.RoundCompleteCount++
	bytes := make([]byte, 32)
	binary.LittleEndian.PutUint64(bytes, m.RoundCompleteCount)
	return bytes, Uint64SummaryValue(m.SummaryValue)
}

func (m *MockTxSystem) Execute(_ *transaction.Transaction) error {
	m.ExecuteCount++
	return nil
}

func (t Uint64SummaryValue) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(uint64(t)))
}

func (t Uint64SummaryValue) Concatenate(left, right state.SummaryValue) state.SummaryValue {
	var s, l, r uint64
	s = uint64(t)
	lSum, ok := left.(Uint64SummaryValue)
	if ok {
		l = uint64(lSum)
	}
	rSum, ok := right.(Uint64SummaryValue)
	if ok {
		r = uint64(rSum)
	}
	return Uint64SummaryValue(s + l + r)
}

func (t Uint64SummaryValue) Bytes() []byte {
	return util.Uint64ToBytes(uint64(t))
}
