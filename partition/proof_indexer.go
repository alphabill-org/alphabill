package partition

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

var (
	ErrIndexNotFound     = errors.New("index not found")
	keyLatestRoundNumber = []byte("latestRoundNumber")
)

type (
	// UnitAndProof read access to state to access unit and unit proofs
	UnitAndProof interface {
		// GetUnit - access tx system unit state
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
		// CreateUnitStateProof - create unit proofs
		CreateUnitStateProof(id types.UnitID, logIndex int) (*types.UnitStateProof, error)
	}

	BlockAndState struct {
		Block *types.Block
		State UnitAndProof
	}

	TxIndex struct {
		RoundNumber  uint64
		TxOrderIndex int
	}

	historyIndex struct {
		UnitProofIndexKeys [][]byte
		TxIndexKey         []byte
	}

	ProofIndexer struct {
		hashAlgorithm crypto.Hash
		storage       keyvaluedb.KeyValueDB
		historySize   uint64 // number of rounds for which the history of unit states is kept
		log           *slog.Logger
		blockCh       chan *BlockAndState

		mu         sync.RWMutex
		ownerUnits map[string][]types.UnitID
	}
)

func NewProofIndexer(algo crypto.Hash, db keyvaluedb.KeyValueDB, historySize uint64, l *slog.Logger) *ProofIndexer {
	return &ProofIndexer{
		hashAlgorithm: algo,
		storage:       db,
		historySize:   historySize,
		log:           l,
		blockCh:       make(chan *BlockAndState, 20),
		ownerUnits:    map[string][]types.UnitID{},
	}
}

func (p *ProofIndexer) Handle(ctx context.Context, block *types.Block, state UnitAndProof) {
	select {
	case <-ctx.Done():
	case p.blockCh <- &BlockAndState{
		Block: block,
		State: state,
	}:
	}
}

func (p *ProofIndexer) GetDB() keyvaluedb.KeyValueDB {
	return p.storage
}

func (p *ProofIndexer) loop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case b := <-p.blockCh:
			roundNumber := b.Block.GetRoundNumber()
			p.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("indexing block %v", roundNumber))
			if err := p.create(ctx, b); err != nil {
				p.log.Warn(fmt.Sprintf("indexing block %v failed", roundNumber), logger.Error(err))
			}
			// clean-up
			if err := p.historyCleanup(ctx, roundNumber); err != nil {
				p.log.Warn("index clean-up failed", logger.Error(err))
			}
		}
	}
}

// create - creates proof index DB entries
func (p *ProofIndexer) create(ctx context.Context, bas *BlockAndState) (err error) {
	if bas.Block.GetRoundNumber() <= p.latestIndexedBlockNumber() {
		return fmt.Errorf("block %d already indexed", bas.Block.GetRoundNumber())
	}
	block := bas.Block
	dbTx, err := p.storage.StartTx()
	if err != nil {
		return fmt.Errorf("start DB transaction failed: %w", err)
	}

	defer func() {
		if err != nil {
			if e := dbTx.Rollback(); e != nil {
				err = errors.Join(err, fmt.Errorf("index transaction rollback failed: %w", e))
			}
		}
		err = dbTx.Commit()
	}()

	var history historyIndex
	for i, tx := range block.Transactions {
		// write down tx index for generating block proofs
		txoHash := tx.TransactionOrder.Hash(p.hashAlgorithm)
		if err = dbTx.Write(txoHash, &TxIndex{
			RoundNumber:  bas.Block.GetRoundNumber(),
			TxOrderIndex: i,
		}); err != nil {
			return err
		}
		history.TxIndexKey = txoHash
		// generate and store proofs for all updated units
		txrHash := tx.Hash(p.hashAlgorithm)
		for _, unitID := range tx.ServerMetadata.TargetUnits {
			var unit *state.Unit
			unit, err = bas.State.GetUnit(unitID, true)
			if err != nil {
				return fmt.Errorf("unit load failed: %w", err)
			}
			unitLogs := unit.Logs()
			p.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("Generating %d proof(s) for unit %X", len(unitLogs), unitID))
			for j, unitLog := range unitLogs {
				if !bytes.Equal(unitLog.TxRecordHash, txrHash) {
					continue
				}
				usp, e := bas.State.CreateUnitStateProof(unitID, j)
				if e != nil {
					err = errors.Join(err, fmt.Errorf("unit %X proof creatioon failed: %w", unitID, e))
					continue
				}
				res, e := state.MarshalUnitData(unitLog.NewUnitData)
				if e != nil {
					err = errors.Join(err, fmt.Errorf("unit %X data encode failed: %w", unitID, e))
					continue
				}
				key := bytes.Join([][]byte{unitID, txoHash}, nil)
				history.UnitProofIndexKeys = append(history.UnitProofIndexKeys, key)
				if err = dbTx.Write(key, &types.UnitDataAndProof{
					UnitData: &types.StateUnitData{Data: res, Bearer: unitLog.NewBearer},
					Proof:    usp,
				}); err != nil {
					return fmt.Errorf("unit proof write failed: %w", err)
				}
			}

			// update owner index
			p.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("Updating owner index for unit %X", unitID))
			if err = p.indexOwner(unitID, unitLogs); err != nil {
				return fmt.Errorf("failed to update owner index: %w", err)
			}
		}
	}
	// update latest round number
	roundNumber := block.GetRoundNumber()
	if err = dbTx.Write(keyLatestRoundNumber, roundNumber); err != nil {
		return fmt.Errorf("round number update failed: %w", err)
	}
	// write delete index
	// only add if there were any transactions
	if len(block.Transactions) > 0 {
		if err = dbTx.Write(util.Uint64ToBytes(roundNumber), history); err != nil {
			return fmt.Errorf("history index write failed: %w", err)
		}
	}
	return nil
}

func (p *ProofIndexer) latestIndexedBlockNumber() uint64 {
	var blockNr uint64
	if found, err := p.storage.Read(keyLatestRoundNumber, &blockNr); !found || err != nil {
		return 0
	}
	return blockNr
}

// historyCleanup - removes old indexes from DB
// todo: NB! it does not currently work correctly if history size is changed
func (p *ProofIndexer) historyCleanup(ctx context.Context, round uint64) (err error) {
	// if history size is set to 0, then do not run clean-up ||
	// if round - history is <= 0 then there is nothing to clean
	if p.historySize == 0 || round-p.historySize <= 0 {
		return nil
	}
	// remove old history
	d := round - p.historySize
	var history historyIndex
	found, err := p.storage.Read(util.Uint64ToBytes(d), &history)
	if err != nil {
		return fmt.Errorf("unable to read delete index: %w", err)
	}
	if !found {
		return nil
	}
	// delete all info added in round
	dbTx, err := p.storage.StartTx()
	if err != nil {
		return fmt.Errorf("unable to start DB transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if e := dbTx.Rollback(); e != nil {
				err = errors.Join(err, fmt.Errorf("history clean rollback failed: %w", e))
			}
		}
		err = dbTx.Commit()
	}()
	if e := dbTx.Delete(history.TxIndexKey); e != nil {
		err = errors.Join(err, fmt.Errorf("unable to delete tx index: %w", e))
	}
	for _, key := range history.UnitProofIndexKeys {
		if e := dbTx.Delete(key); e != nil {
			err = errors.Join(err, fmt.Errorf("unable to delete unit poof index: %w", e))
		}
	}
	p.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("Removed old proofs from block %d, index size %d", d, len(history.UnitProofIndexKeys)))
	return err
}

func (p *ProofIndexer) GetOwnerUnits(ownerID []byte) ([]types.UnitID, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ownerUnits[string(ownerID)], nil
}

func (p *ProofIndexer) indexOwner(unitID types.UnitID, logs []*state.Log) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(logs) == 0 {
		p.log.Error(fmt.Sprintf("cannot index unit owners, unit logs is empty, unitID=%x", unitID))
		return nil
	}
	// logs - tx logs that changed the unit
	// if unit was created in this round:
	//   logs[0] - tx that created the unit
	//   logs[1..n] - txs changing the unit in current round
	// if unit existed before this round:
	//   logs[0] - last tx that changed the unit from previous rounds
	//   logs[1..n] - txs changing the unit in current round
	currOwnerPredicate := logs[len(logs)-1].NewBearer
	if err := p.addOwnerIndex(unitID, currOwnerPredicate); err != nil {
		return fmt.Errorf("failed to add owner index: %w", err)
	}
	if len(logs) > 1 {
		prevOwnerPredicate := logs[0].NewBearer
		if err := p.delOwnerIndex(unitID, prevOwnerPredicate); err != nil {
			return fmt.Errorf("failed to remove owner index: %w", err)
		}
	}
	return nil
}

func (p *ProofIndexer) addOwnerIndex(unitID types.UnitID, ownerPredicate []byte) error {
	ownerID, err := extractOwnerID(ownerPredicate)
	if err != nil {
		return fmt.Errorf("failed to extract owner id: %w", err)
	}
	p.ownerUnits[ownerID] = append(p.ownerUnits[ownerID], unitID)
	return nil
}

func (p *ProofIndexer) delOwnerIndex(unitID types.UnitID, ownerPredicate []byte) error {
	ownerID, err := extractOwnerID(ownerPredicate)
	if err != nil {
		return fmt.Errorf("failed to extract owner id: %w", err)
	}
	unitIDs := p.ownerUnits[ownerID]
	for i, uid := range unitIDs {
		if uid.Eq(unitID) {
			unitIDs = slices.Delete(unitIDs, i, i+1)
			break
		}
	}
	if len(unitIDs) == 0 {
		// no units for owner, delete map key
		delete(p.ownerUnits, ownerID)
	} else {
		// update the removed list
		p.ownerUnits[ownerID] = unitIDs
	}
	return nil
}

// LoadState fills the owner units index from state.
func (p *ProofIndexer) LoadState(s *state.State) error {
	t := &ownerTraverser{ownerUnits: map[string][]types.UnitID{}}
	s.Traverse(t)
	if t.err != nil {
		return fmt.Errorf("failed to traverse state tree: %w", t.err)
	}
	p.ownerUnits = t.ownerUnits
	return nil
}

// ownerTraverser traverses state tree and records all nodes into ownerUnits map
type ownerTraverser struct {
	ownerUnits map[string][]types.UnitID
	err        error
}

func (s *ownerTraverser) Traverse(n *avl.Node[types.UnitID, *state.Unit]) {
	if n == nil || s.err != nil {
		return
	}
	s.Traverse(n.Left())
	s.Traverse(n.Right())

	unit := n.Value()
	ownerID, err := extractOwnerID(unit.Bearer())
	if err != nil {
		s.err = fmt.Errorf("failed to extract owner id: %w", err)
		return
	}
	s.ownerUnits[ownerID] = append(s.ownerUnits[ownerID], n.Key())
}

func ReadTransactionIndex(db keyvaluedb.KeyValueDB, txOrderHash []byte) (*TxIndex, error) {
	index := &TxIndex{}
	f, err := db.Read(txOrderHash, index)
	if err != nil {
		return nil, fmt.Errorf("tx index query failed: %w", err)
	}
	if !f {
		return nil, ErrIndexNotFound
	}
	return index, nil
}

func ReadUnitProofIndex(db keyvaluedb.KeyValueDB, unitID []byte, txOrderHash []byte) (*types.UnitDataAndProof, error) {
	key := bytes.Join([][]byte{unitID, txOrderHash}, nil)
	index := &types.UnitDataAndProof{}
	f, err := db.Read(key, index)
	if err != nil {
		return nil, fmt.Errorf("tx index query failed: %w", err)
	}
	if !f {
		return nil, ErrIndexNotFound
	}
	return index, nil
}

func extractOwnerID(ownerPredicate []byte) (string, error) {
	predicate, err := templates.ExtractPredicate(ownerPredicate)
	if err != nil {
		return "", fmt.Errorf("failed to extract predicate: %w", err)
	}
	var ownerID string
	if predicate.ID == templates.P2pkh256ID {
		p2pkhPredicate, err := templates.ExtractP2pkhPredicate(predicate)
		if err != nil {
			return "", fmt.Errorf("failed to extract p2pkh predicate: %w", err)
		}
		// for p2pkh predicates use pubkey hash as the owner id
		ownerID = string(p2pkhPredicate.PubKeyHash)
	} else {
		// for non-p2pkh predicates use the entire owner predicate as the owner id
		ownerID = string(ownerPredicate)
	}
	return ownerID, nil
}
