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

	FixedState   txsystem.StateReader
	ErrorState   *ErrorState
	FeelessMode  bool
	ExecuteError error
	blockNo      uint64
	uncommitted  bool
	committedUC  *types.UnicityCertificate

	// fee charged for each tx
	Fee uint64

	// mutex guarding all CounterTxSystem fields
	mu sync.Mutex
}

type ErrorState struct {
	txsystem.StateReader
	Err error
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

func (m *CounterTxSystem) StateSummary() (*txsystem.StateSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.uncommitted {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	var c = m.InitCount + m.ExecuteCount
	if m.EndBlockChangesState {
		c += m.EndBlockCount
	}
	return txsystem.NewStateSummary(m.stateCountToHash(c), util.Uint64ToBytes(m.SummaryValue), nil), nil
}

func (m *CounterTxSystem) counter() uint64 {
	var c = m.InitCount + m.ExecuteCount
	if m.EndBlockChangesState {
		c += m.EndBlockCount
	}
	return c
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

func (m *CounterTxSystem) EndBlock() (*txsystem.StateSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.EndBlockCountDelta++
	var state = m.InitCount + m.ExecuteCount + m.ExecuteCountDelta
	if m.EndBlockChangesState {
		m.uncommitted = true
		state += m.EndBlockCount + m.EndBlockCountDelta
	}
	return txsystem.NewStateSummary(m.stateCountToHash(state), util.Uint64ToBytes(m.SummaryValue), nil), nil
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

func (m *CounterTxSystem) SetCommittedUC(committedUC *types.UnicityCertificate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.committedUC = committedUC
}

func (m *CounterTxSystem) SerializeState(w io.Writer) error {
	return m.State().Serialize(w, true, nil)
}

func (m *CounterTxSystem) Execute(tx *types.TransactionOrder) (*types.TransactionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ExecuteCountDelta++

	txBytes, err := tx.MarshalCBOR()
	if err != nil {
		return nil, err
	}

	txr := &types.TransactionRecord{
		Version: 1,
		TransactionOrder: txBytes,
		ServerMetadata: &types.ServerMetadata{
			ActualFee: m.Fee,
			TargetUnits: []types.UnitID{tx.UnitID},
			SuccessIndicator: types.TxStatusSuccessful,
		},
	}
	if m.ExecuteError != nil {
		txr.ServerMetadata.SetError(m.ExecuteError)
		return txr, nil
	}

	m.uncommitted = true
	return txr, nil
}

func (m *CounterTxSystem) IsPermissionedMode() bool {
	return m.FeelessMode
}

func (m *CounterTxSystem) IsFeelessMode() bool {
	return m.FeelessMode
}

func (m *CounterTxSystem) TypeID() types.PartitionTypeID {
	return 999
}

func (m *CounterTxSystem) stateCountToHash(stateCount uint64) []byte {
	if stateCount == 0 {
		return nil
	}
	root := make([]byte, 32)
	binary.LittleEndian.PutUint64(root, stateCount)
	return root
}

// Returns a copy of the txSystem to be used for creating blocks without affecting the original
func (m *CounterTxSystem) Clone() *CounterTxSystem {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &CounterTxSystem{
		InitCount: m.counter(),
		FixedState: m.FixedState,
		Fee: m.Fee,
		EndBlockChangesState: m.EndBlockChangesState,
	}
}

func (m *ErrorState) Serialize(writer io.Writer, committed bool, executedTransactions map[string]uint64) error {
	return m.Err
}

type MockState struct {
	Err error
}

func (m MockState) GetUnit(id types.UnitID, committed bool) (state.Unit, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return &state.UnitV1{}, nil
}

func (m MockState) CreateUnitStateProof(id types.UnitID, logIndex int) (*types.UnitStateProof, error) {
	return &types.UnitStateProof{}, nil
}

func (m MockState) CreateIndex(state.KeyExtractor[string]) (state.Index[string], error) {
	return nil, nil
}

func (m MockState) Serialize(writer io.Writer, committed bool, executedTransactions map[string]uint64) error {
	return nil
}

func (m MockState) GetUnits(unitTypeID *uint32, pdr *types.PartitionDescriptionRecord) ([]types.UnitID, error) {
	return nil, nil
}
