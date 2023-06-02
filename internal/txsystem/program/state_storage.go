package program

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/holiman/uint256"
)

type StateTreeStorage struct {
	state   *rma.Tree
	execCtx *ExecutionContext
}

// CreateStateDataID - creates new program state file ID from program ID and file ID.
// Todo: add task here - There is no specification for this yet, so this is just a quick solution to get up and running
func CreateStateDataID(id *uint256.Int, fileID []byte) *uint256.Int {
	fileIDHash := sha256.Sum256(fileID)
	return uint256.NewInt(0).SetBytes(utiltx.SameShardIDBytes(id, fileIDHash[:]))
}

// NewStateStorage creates an adapter to be used with wasm vm to store and read values from state tree
// Current understanding is that runtime should not have access to other programs state, hence the state id is derived
// from program id (the way it is done is TBD, current implementation is just a quick hack)
func NewStateStorage(state *rma.Tree, eCtx *ExecutionContext) (*StateTreeStorage, error) {
	if state == nil {
		return nil, fmt.Errorf("state tree is nil")
	}
	if eCtx == nil {
		return nil, fmt.Errorf("program execution context is nil")
	}
	return &StateTreeStorage{
		state:   state,
		execCtx: eCtx,
	}, nil
}

func (s *StateTreeStorage) Get(key []byte) ([]byte, error) {
	stateId := CreateStateDataID(s.execCtx.GetProgramID(), key)
	logger.Info("Read state: %X", stateId)
	u, err := s.state.GetUnit(stateId)
	if err != nil {
		return nil, fmt.Errorf("read program state failed, %v", err)
	}
	progData, ok := u.Data.(*Data)
	if !ok {
		return nil, fmt.Errorf("state read failed, invalid type")
	}
	return progData.Bytes(), nil
}

func (s *StateTreeStorage) Put(key []byte, progData []byte) error {
	stateId := CreateStateDataID(s.execCtx.GetProgramID(), key)
	logger.Info("Write state: %X, val %X", stateId, progData)
	_, err := s.state.GetUnit(stateId)
	// call add if no uint found
	if errors.Is(err, rma.ErrUnitNotFound) {
		logger.Debug("Add new state: %X", stateId)
		if err = s.state.AtomicUpdate(rma.AddItem(stateId, script.PredicateAlwaysFalse(), &Data{bytes: progData}, s.execCtx.GetTxHash())); err != nil {
			return fmt.Errorf("failed to add program state to state tree, %w", err)
		}
	} else {
		logger.Debug("Update state: %X", stateId)
		// unit with id is already present, call update
		updateFunc := func(data rma.UnitData) rma.UnitData {
			return &Data{bytes: progData}
		}
		if err = s.state.AtomicUpdate(rma.UpdateData(stateId, updateFunc, s.execCtx.GetTxHash())); err != nil {
			logger.Warning("failed to persist program file")
			return fmt.Errorf("failed to update program state file in state tree, %w", err)
		}
	}
	return nil
}
