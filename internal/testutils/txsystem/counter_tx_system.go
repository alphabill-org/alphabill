package testtxsystem

import (
	"encoding/binary"
	"io"
	"sync"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/state"
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

	FixedState  txsystem.StateReader
	ErrorState  *ErrorState
	blockNo     uint64
	uncommitted bool
	committedUC *types.UnicityCertificate

	// fee charged for each tx
	Fee uint64

	// mutex guarding all CounterTxSystem fields
	mu sync.Mutex
}

type Summary struct {
	root    []byte
	summary []byte
}

type ErrorState struct {
	txsystem.StateReader
	Err error
}

func (s *Summary) Root() []byte {
	return s.root
}

func (s *Summary) Summary() []byte {
	return s.summary
}

func (m *CounterTxSystem) State() txsystem.StateReader {
	if m.FixedState != nil {
		return m.FixedState
	}
	if m.ErrorState != nil {
		return m.ErrorState
	}
	return state.NewEmptyState().Clone()
}

func (m *CounterTxSystem) StateSize() (uint64, error) {
	return 0, nil
}

func (m *CounterTxSystem) StateSummary() (txsystem.StateSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.uncommitted {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	bytes := make([]byte, 32)
	var c = m.InitCount + m.ExecuteCount
	if m.EndBlockChangesState {
		c += m.EndBlockCount
	}
	binary.LittleEndian.PutUint64(bytes, c)
	return &Summary{
		root: bytes, summary: util.Uint64ToBytes(m.SummaryValue),
	}, nil
}

func (m *CounterTxSystem) BeginBlock(nr uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.blockNo = nr
	m.BeginBlockCountDelta++
	m.ExecuteCountDelta = 0
	return nil
}

func (m *CounterTxSystem) Revert() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ExecuteCountDelta = 0
	m.EndBlockCountDelta = 0
	m.BeginBlockCountDelta = 0
	m.RevertCount++
	m.uncommitted = false
}

func (m *CounterTxSystem) EndBlock() (txsystem.StateSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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

func (m *CounterTxSystem) Commit(uc *types.UnicityCertificate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ExecuteCount += m.ExecuteCountDelta
	m.EndBlockCount += m.EndBlockCountDelta
	m.BeginBlockCount += m.BeginBlockCountDelta
	m.uncommitted = false
	m.committedUC = uc
	return nil
}

func (m *CounterTxSystem) CommittedUC() *types.UnicityCertificate {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.committedUC
}

func (m *CounterTxSystem) SerializeState(writer io.Writer, committed bool) error {
	return nil
}

func (m *CounterTxSystem) Execute(tx *types.TransactionOrder) (*types.TransactionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ExecuteCountDelta++
	m.uncommitted = true

	txBytes, err := tx.MarshalCBOR()
	if err != nil {
		return nil, err
	}
	return &types.TransactionRecord{Version: 1, TransactionOrder: txBytes, ServerMetadata: &types.ServerMetadata{ActualFee: m.Fee}}, nil
}

func (m *CounterTxSystem) IsPermissionedMode() bool {
	return false
}

func (m *CounterTxSystem) IsFeelessMode() bool {
	return false
}

func (m *ErrorState) Serialize(writer io.Writer, committed bool) error {
	return m.Err
}
