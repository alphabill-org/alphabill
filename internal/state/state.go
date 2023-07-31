package state

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"sync"

	hasherUtil "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/tree/avl"
)

type (

	// State is a data structure that keeps track of units, unit ledgers, and calculates global state tree root hash.
	//
	// State can be changed by calling Apply function with one or more Action function. Savepoint method can be used
	// to add a special marker to the state that allows all actions that are executed after savepoint was established
	// to be rolled back. In the other words, savepoint lets you roll back part of the state changes instead of the
	// entire state. Releasing a savepoint does NOT trigger a state root hash calculation. To calculate the root hash
	// of the state use method CalculateRoot. Calling a Commit method commits and releases all savepoints.
	State struct {
		mutex         sync.RWMutex
		hashAlgorithm crypto.Hash
		committedTree *avl.Tree[types.UnitID, *Unit]
		savepoints    []*savepoint
	}

	// savepoint is a special marker that allows all actions that are executed after savepoint was established to
	// be rolled back, restoring the state to what it was at the time of the savepoint.
	savepoint = avl.Tree[types.UnitID, *Unit]
)

func NewEmptyState() *State {
	return newEmptySate(loadOptions())
}

// New creates a new state with given options.
func New(opts ...Option) (*State, error) {
	options := loadOptions(opts...)
	s := newEmptySate(options)
	if len(options.actions) > 0 {
		if err := s.Apply(options.actions...); err != nil {
			return nil, err
		}
		if _, _, err := s.CalculateRoot(); err != nil {
			return nil, err
		}
		if err := s.Commit(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func newEmptySate(options *Options) *State {
	hasher := &stateHasher{hashAlgorithm: options.hashAlgorithm}
	tree := avl.NewWithTraverser[types.UnitID, *Unit](hasher)
	return &State{
		hashAlgorithm: options.hashAlgorithm,
		committedTree: tree,
		savepoints:    []*avl.Tree[types.UnitID, *Unit]{tree.Clone()},
	}
}

func (s *State) GetUnit(id types.UnitID, committed bool) (*Unit, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	if committed {
		return s.committedTree.Get(id)
	}
	return s.latestSavepoint().Get(id)
}

func (s *State) AddUnitLog(id types.UnitID, transactionRecordHash []byte) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	u, err := s.latestSavepoint().Get(id)
	if err != nil {
		return 0, fmt.Errorf("unable to add unit log for unit %v: %w", id, err)
	}
	unit := u.Clone()
	logsCount := len(unit.logs)
	l := &log{
		txRecordHash: transactionRecordHash,
		newBearer:    bytes.Clone(unit.bearer),
		newUnitData:  copyData(unit.data),
	}
	if logsCount == 0 {
		// newly created unit
		l.unitLedgerHeadHash = hasherUtil.Sum(s.hashAlgorithm, nil, transactionRecordHash)
	} else {
		// a pre-existing unit
		l.unitLedgerHeadHash = hasherUtil.Sum(s.hashAlgorithm, unit.logs[logsCount-1].unitLedgerHeadHash, transactionRecordHash)
	}
	unit.logs = append(unit.logs, l)
	return len(unit.logs), s.latestSavepoint().Update(id, unit)
}

// Apply applies given actions to the state. All Action functions are executed together as a single atomic operation. If
// any of the Action functions returns an error all previous state changes made by any of the action function will be
// reverted.
func (s *State) Apply(actions ...Action) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.createSavepoint()
	for _, action := range actions {
		if err := action(s.latestSavepoint(), s.hashAlgorithm); err != nil {
			s.rollbackSavepoint()
			return err
		}
	}
	s.releaseSavepoint()
	return nil
}

// Commit commits the state.
func (s *State) Commit() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sp := s.latestSavepoint()
	if !sp.IsClean() {
		return errors.New("call CalculateRoot method before committing a state")
	}
	s.committedTree = sp.Clone()
	s.savepoints = []*savepoint{sp}
	return nil
}

// Revert rolls back all changes made to the state.
func (s *State) Revert() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.savepoints = []*savepoint{s.committedTree.Clone()}
}

// Savepoint creates a new savepoint and returns an id of the savepoint. Use RollbackSavepoint to roll back all
// changes made after calling Savepoint method. Use ReleaseSavepoint to save all changes made to the state.
func (s *State) Savepoint() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.createSavepoint()
}

// RollbackSavepoint destroys a savepoint without keeping the changes in the state tree.
func (s *State) RollbackSavepoint() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.rollbackSavepoint()
}

// ReleaseSavepoint destroys a savepoint, keeping the state changes after it was created. If a savepoint does not exist
// then this method does nothing.
//
// Releasing a savepoint does NOT trigger a state root hash calculation. To calculate the root hash of the state a
// Commit method must be called.
func (s *State) ReleaseSavepoint() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.releaseSavepoint()
}

func (s *State) CalculateRoot() (uint64, []byte, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	sp := s.latestSavepoint()
	if err := sp.Commit(); err != nil {
		return 0, nil, err
	}
	root := sp.Root()
	if root == nil {
		return 0, nil, nil
	}
	value := root.Value()
	return value.subTreeSummaryValue, value.subTreeSummaryHash, nil
}

func (s *State) IsCommitted() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.isCommitted()
}

func (s *State) PruneLog(id types.UnitID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	u, err := s.latestSavepoint().Get(id)
	if err != nil {
		return err
	}
	logSize := len(u.logs)
	if logSize <= 1 {
		return nil
	}
	latestLog := u.logs[logSize-1]
	unit := u.Clone()
	unit.logs = []*log{{
		txRecordHash:       nil,
		unitLedgerHeadHash: bytes.Clone(latestLog.unitLedgerHeadHash),
		newBearer:          bytes.Clone(unit.Bearer()),
		newUnitData:        copyData(unit.Data()),
	}}
	return s.latestSavepoint().Update(id, unit)
}

func (s *State) CreateUnitStateProof(id types.UnitID, logIndex int, uc *types.UnicityCertificate) (*UnitStateProof, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	unit, err := s.committedTree.Get(id)
	if err != nil {
		return nil, fmt.Errorf("unable to get unit %v: %w", id, err)
	}

	if len(unit.logs) < logIndex {
		return nil, fmt.Errorf("invalid unit %v log index: %d", id, logIndex)
	}
	// if unit was created then we do not have a previous unit ledger state hash and this variable is nil.
	var unitLedgerHeadHash []byte
	if logIndex > 0 {
		// existing unit was updated by a transaction
		unitLedgerHeadHash = unit.logs[logIndex-1].unitLedgerHeadHash
	} else if unit.logs[0].txRecordHash == nil {
		// initial state was copied from previous round
		unitLedgerHeadHash = unit.logs[0].unitLedgerHeadHash
	}
	unitTreeCert, err := s.createUnitTreeCert(unit, logIndex)
	if err != nil {
		return nil, fmt.Errorf("unable to extract unit tree cert for unit %v: %w", id, err)
	}
	stateTreeCert, err := s.createStateTreeCert(id)
	if err != nil {
		return nil, fmt.Errorf("unable to extract unit state tree cert for unit %v: %w", id, err)
	}
	var summaryValueInput uint64
	if unit.data != nil {
		summaryValueInput = unit.data.SummaryValueInput()
	}
	return &UnitStateProof{
		unitID:             id,
		previousStateHash:  unitLedgerHeadHash,
		unitTreeCert:       unitTreeCert,
		dataSummary:        summaryValueInput,
		stateTreeCert:      stateTreeCert,
		unicityCertificate: uc,
	}, nil
}

func (s *State) createUnitTreeCert(unit *Unit, logIndex int) (*UnitTreeCert, error) {
	merkle := mt.New(s.hashAlgorithm, unit.logs)
	path, err := merkle.GetMerklePath(logIndex)
	if err != nil {
		return nil, err
	}
	l := unit.logs[logIndex]
	dataHasher := s.hashAlgorithm.New()
	dataHasher.Write(l.newBearer)
	l.newUnitData.Write(dataHasher)
	return &UnitTreeCert{
		transactionRecordHash: l.txRecordHash,
		unitDataHash:          dataHasher.Sum(nil),
		path:                  path,
	}, nil
}

func (s *State) createStateTreeCert(id types.UnitID) (*StateTreeCert, error) {
	var path []*StateTreePathItem
	node := s.committedTree.Root()
	for node != nil && !id.Eq(node.Key()) {
		nodeKey := node.Key()
		v := getSummaryValueInput(node)
		var item *StateTreePathItem
		if id.Compare(nodeKey) == -1 {
			nodeRight := node.Right()
			item = &StateTreePathItem{
				id:                  nodeKey,
				hash:                getSubTreeLogRootHash(node),
				nodeSummaryInput:    v,
				siblingHash:         getSubTreeSummaryHash(nodeRight),
				subTreeSummaryValue: getSubTreeSummaryValue(nodeRight),
			}
			node = node.Left()
		} else {
			nodeLeft := node.Left()
			item = &StateTreePathItem{
				id:                  nodeKey,
				hash:                getSubTreeLogRootHash(node),
				nodeSummaryInput:    v,
				siblingHash:         getSubTreeSummaryHash(nodeLeft),
				subTreeSummaryValue: getSubTreeSummaryValue(nodeLeft),
			}
			node = node.Right()
		}
		path = append([]*StateTreePathItem{item}, path...)
	}
	if id.Eq(node.Key()) {
		nodeLeft := node.Left()
		nodeRight := node.Right()
		return &StateTreeCert{
			leftHash:          getSubTreeSummaryHash(nodeLeft),
			leftSummaryValue:  getSubTreeSummaryValue(nodeLeft),
			rightHash:         getSubTreeSummaryHash(nodeRight),
			rightSummaryValue: getSubTreeSummaryValue(nodeRight),
			path:              path,
		}, nil
	}
	return nil, fmt.Errorf("unable to extract unit state tree cert for unit %v", id)
}

func (s *State) createSavepoint() {
	s.savepoints = append(s.savepoints, s.latestSavepoint().Clone())
}

func (s *State) rollbackSavepoint() {
	c := len(s.savepoints)
	if c == 1 {
		s.savepoints = []*savepoint{s.committedTree.Clone()}
		return
	}
	s.savepoints = s.savepoints[0 : c-1]
}

func (s *State) releaseSavepoint() {
	c := len(s.savepoints)
	if c == 1 {
		return
	}
	s.savepoints[c-2] = s.latestSavepoint()
	s.savepoints = s.savepoints[0 : c-1]
}

func (s *State) isCommitted() bool {
	return len(s.savepoints) == 1 && s.savepoints[0].IsClean()
}

// latestSavepoint returns the latest savepoint.
func (s *State) latestSavepoint() *savepoint {
	l := len(s.savepoints)
	return s.savepoints[l-1]
}

func getSubTreeLogRootHash(n *avl.Node[types.UnitID, *Unit]) []byte {
	if n == nil || n.Value() == nil {
		return nil
	}
	return n.Value().logRoot
}

func getSubTreeSummaryValue(n *avl.Node[types.UnitID, *Unit]) uint64 {
	if n == nil || n.Value() == nil {
		return 0
	}
	return n.Value().subTreeSummaryValue
}

func getSummaryValueInput(node *avl.Node[types.UnitID, *Unit]) uint64 {
	if node == nil || node.Value() == nil || node.Value().data == nil {
		return 0
	}
	return node.Value().data.SummaryValueInput()
}

func getSubTreeSummaryHash(node *avl.Node[types.UnitID, *Unit]) []byte {
	if node == nil || node.Value() == nil {
		return nil
	}
	return node.Value().subTreeSummaryHash
}
