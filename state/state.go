package state

import (
	"bytes"
	"crypto"
	"fmt"
	"io"
	"sync"

	hasherUtil "github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/tree/mt"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
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
		mutex           sync.RWMutex
		hashAlgorithm   crypto.Hash
		committedTree   *tree
		committedTreeUC *types.UnicityCertificate

		// savepoint is a special marker that allows all actions that are executed after tree was established to
		// be rolled back, restoring the state to what it was at the time of the tree.
		savepoints []*tree
	}

	tree = avl.Tree[types.UnitID, *Unit]
	node = avl.Node[types.UnitID, *Unit]

	// UnitDataConstructor is a function that constructs an empty UnitData structure based on UnitID
	UnitDataConstructor func(types.UnitID) (UnitData, error)
)

func NewEmptyState(opts ...Option) *State {
	options := loadOptions(opts...)

	hasher := newStateHasher(options.hashAlgorithm)
	t := avl.NewWithTraverser[types.UnitID, *Unit](hasher)

	return &State{
		hashAlgorithm: options.hashAlgorithm,
		committedTree: t,
		savepoints:    []*tree{t.Clone()},
	}
}

func NewRecoveredState(stateData io.Reader, udc UnitDataConstructor, opts ...Option) (*State, error) {
	options := loadOptions(opts...)
	if stateData == nil {
		return nil, fmt.Errorf("reader is nil")
	}
	if udc == nil {
		return nil, fmt.Errorf("unit data constructor is nil")
	}

	crc32Reader := NewCRC32Reader(stateData, CBORChecksumLength)
	decoder := cbor.NewDecoder(crc32Reader)

	var header Header
	err := decoder.Decode(&header)
	if err != nil {
		return nil, fmt.Errorf("unable to decode header: %w", err)
	}

	root, err := readNodeRecords(decoder, udc, header.NodeRecordCount, options.hashAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("unable to decode node records: %w", err)
	}

	var checksum []byte
	if err = decoder.Decode(&checksum); err != nil {
		return nil, fmt.Errorf("unable to decode checksum: %w", err)
	}
	if util.BytesToUint32(checksum) != crc32Reader.Sum() {
		return nil, fmt.Errorf("checksum mismatch")
	}

	hasher := newStateHasher(options.hashAlgorithm)
	t := avl.NewWithTraverserAndRoot[types.UnitID, *Unit](hasher, root)
	state := &State{
		hashAlgorithm: options.hashAlgorithm,
		savepoints:    []*tree{t},
	}
	if _, _, err := state.CalculateRoot(); err != nil {
		return nil, err
	}
	if header.UnicityCertificate != nil {
		if err := state.Commit(header.UnicityCertificate); err != nil {
			return nil, fmt.Errorf("unable to commit recovered state: %w", err)
		}
	}

	return state, nil
}

func readNodeRecords(decoder *cbor.Decoder, unitDataConstructor UnitDataConstructor, count uint64, hashAlgorithm crypto.Hash) (*node, error) {
	if count == 0 {
		return nil, nil
	}

	var nodeStack util.Stack[*node]
	for i := uint64(0); i < count; i++ {
		var nodeRecord nodeRecord
		err := decoder.Decode(&nodeRecord)
		if err != nil {
			return nil, fmt.Errorf("unable to decode node record: %w", err)
		}

		unitData, err := unitDataConstructor(nodeRecord.UnitID)
		if err != nil {
			return nil, fmt.Errorf("unable to construct unit data: %w", err)
		}

		err = cbor.Unmarshal(nodeRecord.UnitData, &unitData)
		if err != nil {
			return nil, fmt.Errorf("unable to decode unit data: %w", err)
		}

		latestLog := &Log{
			UnitLedgerHeadHash: nodeRecord.UnitLedgerHeadHash,
			NewBearer:          nodeRecord.OwnerCondition,
			NewUnitData:        unitData,
		}
		logRoot := mt.EvalMerklePath(nodeRecord.UnitTreePath, latestLog, hashAlgorithm)

		unit := &Unit{logRoot: logRoot}
		if len(nodeRecord.UnitTreePath) > 0 {
			// A non-zero UnitTreePath length means that the unit had multiple logs at serialization.
			// Those logs must be pruned at the beginning of the next round and the summary hash must
			// be recalculated for such units after pruning. Let's add an extra empty log for the unit,
			// so that the pruner can find it and the summary hash is recalculated. This does not
			// interfere with proof indexer as proofs are not calculated for the recovered round.
			// Everything else uses just the latest log.
			unit.logs = []*Log{{}, latestLog}
		} else {
			unit.logs = []*Log{latestLog}
		}

		var right, left *node
		if nodeRecord.HasRight {
			right = nodeStack.Pop()
		}
		if nodeRecord.HasLeft {
			left = nodeStack.Pop()
		}

		nodeStack.Push(avl.NewBalancedNode(nodeRecord.UnitID, unit, left, right))
	}

	root := nodeStack.Pop()
	if !nodeStack.IsEmpty() {
		return nil, fmt.Errorf("%d unexpected node record(s)", len(nodeStack))
	}
	return root, nil
}

// Clone returns a clone of the state. The original state and the cloned state can be used by different goroutines but
// can never be merged. The cloned state is usually used by read only operations (e.g. unit proof generation).
func (s *State) Clone() *State {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return &State{
		hashAlgorithm:   s.hashAlgorithm,
		committedTree:   s.committedTree.Clone(),
		committedTreeUC: s.committedTreeUC,
		savepoints:      []*tree{s.latestSavepoint().Clone()},
	}
}

func (s *State) GetUnit(id types.UnitID, committed bool) (*Unit, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	if committed {
		return s.committedTree.Get(id)
	}
	u, err := s.latestSavepoint().Get(id)
	if err != nil {
		return nil, err
	}
	return u.Clone(), nil
}

func (s *State) AddUnitLog(id types.UnitID, transactionRecordHash []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	u, err := s.latestSavepoint().Get(id)
	if err != nil {
		return fmt.Errorf("unable to add unit log for unit %v: %w", id, err)
	}
	unit := u.Clone()
	logsCount := len(unit.logs)
	l := &Log{
		TxRecordHash: transactionRecordHash,
		NewBearer:    bytes.Clone(unit.bearer),
		NewUnitData:  copyData(unit.data),
	}
	if logsCount == 0 {
		// newly created unit
		l.UnitLedgerHeadHash = hasherUtil.Sum(s.hashAlgorithm, nil, transactionRecordHash)
	} else {
		// a pre-existing unit
		l.UnitLedgerHeadHash = hasherUtil.Sum(s.hashAlgorithm, unit.logs[logsCount-1].UnitLedgerHeadHash, transactionRecordHash)
	}
	unit.logs = append(unit.logs, l)
	return s.latestSavepoint().Update(id, unit)
}

// Apply applies given actions to the state. All Action functions are executed together as a single atomic operation. If
// any of the Action functions returns an error all previous state changes made by any of the action function will be
// reverted.
func (s *State) Apply(actions ...Action) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	id := s.createSavepoint()
	for _, action := range actions {
		if err := action(s.latestSavepoint(), s.hashAlgorithm); err != nil {
			s.rollbackToSavepoint(id)
			return err
		}
	}
	s.releaseToSavepoint(id)
	return nil
}

// Commit commits the changes in the latest savepoint.
func (s *State) Commit(uc *types.UnicityCertificate) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sp := s.latestSavepoint()
	if !sp.IsClean() {
		return fmt.Errorf("call CalculateRoot method before committing a state")
	}

	// Verify that the uc certifies the state being committed
	var summaryValue uint64
	var summaryHash []byte
	if sp.Root() != nil {
		summaryValue = sp.Root().Value().subTreeSummaryValue
		summaryHash = sp.Root().Value().subTreeSummaryHash
	} else {
		summaryHash = make([]byte, s.hashAlgorithm.Size())
	}

	if !bytes.Equal(uc.InputRecord.Hash, summaryHash) {
		return fmt.Errorf("state summary hash is not equal to the summary hash in UC")
	}

	if !bytes.Equal(uc.InputRecord.SummaryValue, util.Uint64ToBytes(summaryValue)) {
		return fmt.Errorf("state summary value is not equal to the summary value in UC")
	}

	s.committedTree = sp.Clone()
	s.committedTreeUC = uc
	s.savepoints = []*tree{sp}
	return nil
}

// CommittedUC returns the Unicity Certificate of the committed state.
func (s *State) CommittedUC() *types.UnicityCertificate {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.committedTreeUC
}

// Revert rolls back all changes made to the state.
func (s *State) Revert() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.savepoints = []*tree{s.committedTree.Clone()}
}

// Savepoint creates a new savepoint and returns an id of the savepoint. Use RollbackToSavepoint to roll back all
// changes made after calling Savepoint method. Use ReleaseToSavepoint to save all changes made to the state.
func (s *State) Savepoint() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.createSavepoint()
}

// RollbackToSavepoint destroys savepoints without keeping the changes in the state tree. All actions that were executed
// after the savepoint was established are rolled back, restoring the state to what it was at the time of the savepoint.
func (s *State) RollbackToSavepoint(id int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.rollbackToSavepoint(id)
}

// ReleaseToSavepoint destroys all savepoints, keeping all state changes after it was created. If a savepoint with given
// id does not exist then this method does nothing.
//
// Releasing savepoints does NOT trigger a state root hash calculation. To calculate the root hash of the state a
// Commit method must be called.
func (s *State) ReleaseToSavepoint(id int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.releaseToSavepoint(id)
}

func (s *State) CalculateRoot() (uint64, []byte, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	sp := s.latestSavepoint()
	sp.Commit()
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

func (s *State) Prune() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	sp := s.latestSavepoint()
	pruner := newStatePruner(sp)
	sp.Traverse(pruner)
	return pruner.Err()
}

// Serialize writes the current committed state to the given writer.
// Not concurrency safe. Should clone the state before calling this.
func (s *State) Serialize(writer io.Writer, header *Header, committed bool) error {
	crc32Writer := NewCRC32Writer(writer)
	encoder := cbor.NewEncoder(crc32Writer)

	var tree *tree
	if committed {
		tree = s.committedTree
		header.UnicityCertificate = s.committedTreeUC
	} else {
		tree = s.latestSavepoint()
	}

	// Add node record count to header
	snc := NewStateNodeCounter()
	tree.Traverse(snc)
	header.NodeRecordCount = snc.NodeCount()

	// Write header
	if err := encoder.Encode(header); err != nil {
		return fmt.Errorf("unable to write header: %w", err)
	}

	// Write node records
	ss := newStateSerializer(encoder, s.hashAlgorithm)
	if tree.Traverse(ss); ss.err != nil {
		return fmt.Errorf("unable to write node records: %w", ss.err)
	}

	// Write checksum (as a fixed length byte array for easier decoding)
	if err := encoder.Encode(util.Uint32ToBytes(crc32Writer.Sum())); err != nil {
		return fmt.Errorf("unable to write checksum: %w", err)
	}

	return nil
}

func (s *State) CreateUnitStateProof(id types.UnitID, logIndex int) (*types.UnitStateProof, error) {
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
		unitLedgerHeadHash = unit.logs[logIndex-1].UnitLedgerHeadHash
	} else if unit.logs[0].TxRecordHash == nil {
		// initial state was copied from previous round
		unitLedgerHeadHash = unit.logs[0].UnitLedgerHeadHash
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

	// TODO verify proof before returning
	return &types.UnitStateProof{
		UnitID:             id,
		PreviousStateHash:  unitLedgerHeadHash,
		UnitTreeCert:       unitTreeCert,
		DataSummary:        summaryValueInput,
		StateTreeCert:      stateTreeCert,
		UnicityCertificate: s.committedTreeUC,
	}, nil
}

func (s *State) HashAlgorithm() crypto.Hash {
	return s.hashAlgorithm
}

func (s *State) Traverse(traverser avl.Traverser[types.UnitID, *Unit]) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	s.committedTree.Traverse(traverser)
}

func (s *State) createUnitTreeCert(unit *Unit, logIndex int) (*types.UnitTreeCert, error) {
	merkle := mt.New(s.hashAlgorithm, unit.logs)
	path, err := merkle.GetMerklePath(logIndex)
	if err != nil {
		return nil, err
	}
	l := unit.logs[logIndex]
	dataHasher := s.hashAlgorithm.New()
	dataHasher.Write(l.NewBearer)
	if err = l.NewUnitData.Write(dataHasher); err != nil {
		return nil, fmt.Errorf("add to hasher error: %w", err)
	}
	return &types.UnitTreeCert{
		TransactionRecordHash: l.TxRecordHash,
		UnitDataHash:          dataHasher.Sum(nil),
		Path:                  path,
	}, nil
}

func (s *State) createStateTreeCert(id types.UnitID) (*types.StateTreeCert, error) {
	var path []*types.StateTreePathItem
	node := s.committedTree.Root()
	for node != nil && !id.Eq(node.Key()) {
		nodeKey := node.Key()
		v := getSummaryValueInput(node)
		var item *types.StateTreePathItem
		if id.Compare(nodeKey) == -1 {
			nodeRight := node.Right()
			item = &types.StateTreePathItem{
				ID:                  nodeKey,
				Hash:                getSubTreeLogRootHash(node),
				NodeSummaryInput:    v,
				SiblingHash:         getSubTreeSummaryHash(nodeRight),
				SubTreeSummaryValue: getSubTreeSummaryValue(nodeRight),
			}
			node = node.Left()
		} else {
			nodeLeft := node.Left()
			item = &types.StateTreePathItem{
				ID:                  nodeKey,
				Hash:                getSubTreeLogRootHash(node),
				NodeSummaryInput:    v,
				SiblingHash:         getSubTreeSummaryHash(nodeLeft),
				SubTreeSummaryValue: getSubTreeSummaryValue(nodeLeft),
			}
			node = node.Right()
		}
		path = append([]*types.StateTreePathItem{item}, path...)
	}
	if id.Eq(node.Key()) {
		nodeLeft := node.Left()
		nodeRight := node.Right()
		return &types.StateTreeCert{
			LeftHash:          getSubTreeSummaryHash(nodeLeft),
			LeftSummaryValue:  getSubTreeSummaryValue(nodeLeft),
			RightHash:         getSubTreeSummaryHash(nodeRight),
			RightSummaryValue: getSubTreeSummaryValue(nodeRight),
			Path:              path,
		}, nil
	}
	return nil, fmt.Errorf("unable to extract unit state tree cert for unit %v", id)
}

func (s *State) createSavepoint() int {
	clonedSavepoint := s.latestSavepoint().Clone()
	// mark AVL Tree nodes as clean
	clonedSavepoint.Traverse(&avl.PostOrderCommitTraverser[types.UnitID, *Unit]{})
	s.savepoints = append(s.savepoints, clonedSavepoint)
	return len(s.savepoints) - 1
}

func (s *State) rollbackToSavepoint(id int) {
	c := len(s.savepoints)
	if id > c {
		// nothing to revert
		return
	}
	s.savepoints = s.savepoints[0:id]
}

func (s *State) releaseToSavepoint(id int) {
	c := len(s.savepoints)
	if id > c {
		// nothing to release
		return
	}
	s.savepoints[id-1] = s.latestSavepoint()
	s.savepoints = s.savepoints[0:id]
}

func (s *State) isCommitted() bool {
	return len(s.savepoints) == 1 &&
		s.savepoints[0].IsClean() &&
		isRootClean(s.savepoints[0]) &&
		s.committedTreeUC != nil
}

func isRootClean(s *tree) bool {
	root := s.Root()
	if root == nil {
		return true
	}
	return root.Value().summaryCalculated
}

// latestSavepoint returns the latest savepoint.
func (s *State) latestSavepoint() *tree {
	l := len(s.savepoints)
	return s.savepoints[l-1]
}

func getSummaryValueInput(n *node) uint64 {
	if n == nil || n.Value() == nil || n.Value().data == nil {
		return 0
	}
	return n.Value().data.SummaryValueInput()
}

func getSubTreeSummaryValue(n *node) uint64 {
	if n == nil || n.Value() == nil {
		return 0
	}
	return n.Value().subTreeSummaryValue
}

func getSubTreeLogRootHash(n *node) []byte {
	if n == nil || n.Value() == nil {
		return nil
	}
	return n.Value().logRoot
}

func getSubTreeSummaryHash(n *node) []byte {
	if n == nil || n.Value() == nil {
		return nil
	}
	return n.Value().subTreeSummaryHash
}
