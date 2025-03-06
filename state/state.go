package state

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"io"
	"sync"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/tree/mt"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/tree/avl"
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

	Unit interface {
		types.Versioned
		avl.Value[Unit]

		Data() types.UnitData
	}

	tree = avl.Tree[types.UnitID, Unit]
	node = avl.Node[types.UnitID, Unit]

	// UnitDataConstructor is a function that constructs an empty UnitData structure based on UnitID
	UnitDataConstructor func(types.UnitID) (types.UnitData, error)
)

func NewEmptyState(opts ...Option) *State {
	options := loadOptions(opts...)

	hasher := newStateHasher(options.hashAlgorithm)
	t := avl.NewWithTraverser[types.UnitID, Unit](hasher)

	return &State{
		hashAlgorithm: options.hashAlgorithm,
		committedTree: t,
		savepoints:    []*tree{t.Clone()},
	}
}

func NewRecoveredState(stateData io.Reader, udc UnitDataConstructor, opts ...Option) (*State, error) {
	if stateData == nil {
		return nil, fmt.Errorf("reader is nil")
	}
	if udc == nil {
		return nil, fmt.Errorf("unit data constructor is nil")
	}
	return readState(stateData, udc, opts...)
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

		err = types.Cbor.Unmarshal(nodeRecord.UnitData, &unitData)
		if err != nil {
			return nil, fmt.Errorf("unable to decode unit data: %w", err)
		}

		latestLog := &Log{
			UnitLedgerHeadHash: nodeRecord.UnitLedgerHeadHash,
			NewUnitData:        unitData,
		}
		logsHash, err := mt.EvalMerklePath(nodeRecord.UnitTreePath, latestLog, hashAlgorithm)
		if err != nil {
			return nil, fmt.Errorf("unable to evaluate merkle path: %w", err)
		}

		unit := &UnitV1{logsHash: logsHash}
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

		nodeStack.Push(avl.NewBalancedNode(nodeRecord.UnitID, Unit(unit), left, right))
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

func (s *State) GetUnit(id types.UnitID, committed bool) (Unit, error) {
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
	unit, err := ToUnitV1(u.Clone())
	if err != nil {
		return fmt.Errorf("add log failed for unit %v: %w", id, err)
	}
	logsCount := len(unit.logs)
	l := &Log{
		TxRecordHash:   transactionRecordHash,
		NewUnitData:    copyData(unit.data),
		NewStateLockTx: bytes.Clone(unit.stateLockTx),
	}
	if logsCount == 0 {
		// newly created unit
		l.UnitLedgerHeadHash, err = abhash.HashValues(s.hashAlgorithm, nil, transactionRecordHash)
	} else {
		// a pre-existing unit
		l.UnitLedgerHeadHash, err = abhash.HashValues(s.hashAlgorithm, unit.logs[logsCount-1].UnitLedgerHeadHash, transactionRecordHash)
	}
	if err != nil {
		return fmt.Errorf("unable to hash unit ledger head: %w", err)
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
	id, err := s.createSavepoint()
	if err != nil {
		return fmt.Errorf("unable to create savepoint: %w", err)
	}
	for _, action := range actions {
		if err := action(s.latestSavepoint(), s.hashAlgorithm); err != nil {
			s.rollbackToSavepoint(id)
			return err
		}
	}
	s.releaseToSavepoint(id)
	return nil
}

// Commit makes the changes in the latest savepoint permanent.
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
		unit, err := ToUnitV1(sp.Root().Value())
		if err != nil {
			return fmt.Errorf("unable to get root unit: %w", err)
		}
		summaryValue = unit.subTreeSummaryValue
		summaryHash = unit.subTreeSummaryHash
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
func (s *State) Savepoint() (int, error) {
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
	err := sp.Commit()
	if err != nil {
		return 0, nil, fmt.Errorf("unable to commit savepoint: %w", err)
	}
	root := sp.Root()
	if root == nil {
		return 0, nil, nil
	}
	value, err := ToUnitV1(root.Value())
	if err != nil {
		return 0, nil, fmt.Errorf("unable to get root unit: %w", err)
	}
	return value.subTreeSummaryValue, value.subTreeSummaryHash, nil
}

func (s *State) IsCommitted() (bool, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.isCommitted()
}

func (s *State) Prune() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	sp := s.latestSavepoint()
	pruner := newStatePruner(sp)
	return sp.Traverse(pruner)
}

func (s *State) Size() (uint64, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	ss := &stateSize{}
	return ss.size, s.latestSavepoint().Traverse(ss)
}

// Serialize writes the current committed state to the given writer.
// Not concurrency safe. Should clone the state before calling this.
func (s *State) Serialize(writer io.Writer, committed bool) error {
	crc32Writer := NewCRC32Writer(writer)
	encoder, err := types.Cbor.GetEncoder(crc32Writer)
	if err != nil {
		return fmt.Errorf("unable to get encoder: %w", err)
	}

	header := &header{Version: 1}

	var tree *tree
	if committed {
		tree = s.committedTree
		header.UnicityCertificate = s.committedTreeUC
	} else {
		tree = s.latestSavepoint()
	}

	// Add node record count to header
	snc := NewStateNodeCounter()
	if err := tree.Traverse(snc); err != nil {
		return fmt.Errorf("unable to count node records: %w", err)
	}
	header.NodeRecordCount = snc.NodeCount()

	// Write header
	if err := encoder.Encode(header); err != nil {
		return fmt.Errorf("unable to write header: %w", err)
	}

	// Write node records
	ss := newStateSerializer(encoder.Encode, s.hashAlgorithm)
	if err := tree.Traverse(ss); err != nil {
		return fmt.Errorf("unable to write node records: %w", err)
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
	u, err := s.committedTree.Get(id)
	if err != nil {
		return nil, fmt.Errorf("unable to get unit %v: %w", id, err)
	}

	unit, err := ToUnitV1(u)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the unit: %w", err)
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
	ucBytes, err := s.committedTreeUC.MarshalCBOR()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal unicity certificate: %w", err)
	}
	return &types.UnitStateProof{
		UnitID:             id,
		UnitLedgerHash:     unitLedgerHeadHash,
		UnitTreeCert:       unitTreeCert,
		UnitValue:          summaryValueInput,
		StateTreeCert:      stateTreeCert,
		UnicityCertificate: ucBytes,
	}, nil
}

func (s *State) HashAlgorithm() crypto.Hash {
	return s.hashAlgorithm
}

func (s *State) Traverse(traverser avl.Traverser[types.UnitID, Unit]) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.committedTree.Traverse(traverser)
}

func (s *State) GetUnits(unitTypeID *uint32, pdr *types.PartitionDescriptionRecord) ([]types.UnitID, error) {
	if pdr == nil && unitTypeID != nil {
		return nil, errors.New("partition description record is nil")
	}
	traverser := NewFilter(func(unitID types.UnitID, unit Unit) (bool, error) {
		// get all units if no unit type is provided
		if unitTypeID == nil {
			return true, nil
		}
		// filter by type if unit type is provided
		unitIDType, err := pdr.ExtractUnitType(unitID)
		if err != nil {
			return false, fmt.Errorf("extracting unit type from unit ID: %w", err)
		}
		return unitIDType == *unitTypeID, nil
	})
	if err := s.Traverse(traverser); err != nil {
		return nil, fmt.Errorf("failed to traverse state: %w", err)
	}
	return traverser.filteredUnitIDs, nil
}

func (s *State) createUnitTreeCert(unit *UnitV1, logIndex int) (*types.UnitTreeCert, error) {
	merkle, err := mt.New(s.hashAlgorithm, unit.logs)
	if err != nil {
		return nil, fmt.Errorf("unable to create merkle tree: %w", err)
	}
	path, err := merkle.GetMerklePath(logIndex)
	if err != nil {
		return nil, err
	}
	l := unit.logs[logIndex]
	dataHasher := abhash.New(s.hashAlgorithm.New())
	l.NewUnitData.Write(dataHasher)
	h, err := dataHasher.Sum()
	if err != nil {
		return nil, fmt.Errorf("unable to hash unit data: %w", err)
	}
	return &types.UnitTreeCert{
		TransactionRecordHash: l.TxRecordHash,
		UnitDataHash:          h,
		Path:                  path,
	}, nil
}

func (s *State) createStateTreeCert(id types.UnitID) (*types.StateTreeCert, error) {
	getStateTreePathItem := func(n *node, child *node, summaryValueInput uint64, nodeKey types.UnitID) (*types.StateTreePathItem, error) {
		logsHash, err := getSubTreeLogsHash(n)
		if err != nil {
			return nil, fmt.Errorf("unable to extract logs hash for unit %s: %w", id, err)
		}
		siblingSummaryHash, err := getSubTreeSummaryHash(child)
		if err != nil {
			return nil, fmt.Errorf("unable to extract sibling summary hash for unit %s: %w", id, err)
		}
		siblingSummaryValue, err := getSubTreeSummaryValue(child)
		if err != nil {
			return nil, fmt.Errorf("unable to extract sibling summary value for unit %s: %w", id, err)
		}
		return &types.StateTreePathItem{
			UnitID:              nodeKey,
			LogsHash:            logsHash,
			Value:               summaryValueInput,
			SiblingSummaryHash:  siblingSummaryHash,
			SiblingSummaryValue: siblingSummaryValue,
		}, nil
	}

	var path []*types.StateTreePathItem
	n := s.committedTree.Root()
	for n != nil && !id.Eq(n.Key()) {
		nodeKey := n.Key()
		v, err := getSummaryValueInput(n)
		if err != nil {
			return nil, fmt.Errorf("unable to extract summary value input for unit %s: %w", id, err)
		}
		var item *types.StateTreePathItem

		if id.Compare(nodeKey) == -1 {
			item, err = getStateTreePathItem(n, n.Right(), v, nodeKey)
			n = n.Left()
		} else {
			item, err = getStateTreePathItem(n, n.Left(), v, nodeKey)
			n = n.Right()
		}
		if err != nil {
			return nil, err
		}
		path = append([]*types.StateTreePathItem{item}, path...)
	}
	if id.Eq(n.Key()) {
		nodeLeft := n.Left()
		nodeRight := n.Right()
		lv, lh, err := getSubTreeSummary(nodeLeft)
		if err != nil {
			return nil, fmt.Errorf("unable to extract left subtree summary for unit %s: %w", id, err)
		}
		rv, rh, err := getSubTreeSummary(nodeRight)
		if err != nil {
			return nil, fmt.Errorf("unable to extract right subtree summary for unit %s: %w", id, err)
		}
		return &types.StateTreeCert{
			LeftSummaryHash:   lh,
			LeftSummaryValue:  lv,
			RightSummaryHash:  rh,
			RightSummaryValue: rv,
			Path:              path,
		}, nil
	}
	return nil, fmt.Errorf("unable to extract unit state tree cert for unit %v", id)
}

func (s *State) createSavepoint() (int, error) {
	clonedSavepoint := s.latestSavepoint().Clone()
	// mark AVL Tree nodes as clean
	err := clonedSavepoint.Traverse(&avl.PostOrderCommitTraverser[types.UnitID, Unit]{})
	if err != nil {
		return 0, fmt.Errorf("unable to mark the tree clean: %w", err)
	}
	s.savepoints = append(s.savepoints, clonedSavepoint)
	return len(s.savepoints) - 1, nil
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

// TODO: a better name perhaps, committed state and committed tree are different things, this checks if tree is committed/clean
func (s *State) isCommitted() (bool, error) {
	if len(s.savepoints) == 1 && s.savepoints[0].IsClean() {
		return isRootClean(s.savepoints[0])
	}
	return false, nil
}

func isRootClean(s *tree) (bool, error) {
	root := s.Root()
	if root == nil {
		return true, nil
	}
	unit, err := ToUnitV1(root.Value())
	if err != nil {
		return false, err
	}
	return unit.summaryCalculated, nil
}

// latestSavepoint returns the latest savepoint.
func (s *State) latestSavepoint() *tree {
	l := len(s.savepoints)
	return s.savepoints[l-1]
}

func getSummaryValueInput(n *node) (uint64, error) {
	if n == nil || n.Value() == nil {
		return 0, nil
	}
	u, err := ToUnitV1(n.Value())
	if err != nil {
		return 0, err
	}
	if u.data == nil {
		return 0, nil
	}
	return u.data.SummaryValueInput(), nil
}

func getSubTreeSummaryValue(n *node) (uint64, error) {
	if n == nil || n.Value() == nil {
		return 0, nil
	}
	u, err := ToUnitV1(n.Value())
	if err != nil {
		return 0, err
	}
	return u.subTreeSummaryValue, nil
}

func getSubTreeLogsHash(n *node) ([]byte, error) {
	if n == nil || n.Value() == nil {
		return nil, nil
	}
	u, err := ToUnitV1(n.Value())
	if err != nil {
		return nil, err
	}
	return u.logsHash, nil
}

func getSubTreeSummaryHash(n *node) ([]byte, error) {
	if n == nil || n.Value() == nil {
		return nil, nil
	}
	u, err := ToUnitV1(n.Value())
	if err != nil {
		return nil, err
	}
	return u.subTreeSummaryHash, nil
}

func getSubTreeSummary(n *node) (uint64, []byte, error) {
	if n == nil || n.Value() == nil {
		return 0, nil, nil
	}
	u, err := ToUnitV1(n.Value())
	if err != nil {
		return 0, nil, err
	}
	return u.subTreeSummaryValue, u.subTreeSummaryHash, nil
}
