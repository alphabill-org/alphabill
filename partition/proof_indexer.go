package partition

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/state"
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
	}

	ProofIndexer struct {
		hashAlgorithm crypto.Hash
		storage       keyvaluedb.KeyValueDB
		historySize   uint64 // number of rounds for which the history of unit states is kept
		log           *slog.Logger
		blockCh       chan *BlockAndState
	}
)

func NewProofIndexer(algo crypto.Hash, db keyvaluedb.KeyValueDB, historySize uint64, l *slog.Logger) *ProofIndexer {
	return &ProofIndexer{
		hashAlgorithm: algo,
		storage:       db,
		historySize:   historySize,
		log:           l,
		blockCh:       make(chan *BlockAndState, 20),
	}
}

func (p *ProofIndexer) IndexBlock(ctx context.Context, block *types.Block, state UnitAndProof) error {
	roundNumber := block.GetRoundNumber()
	p.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("indexing block %v", roundNumber))
	if err := p.create(ctx, block, state); err != nil {
		return fmt.Errorf("creating index failed: %w", err)
	}
	// clean-up
	if err := p.historyCleanup(ctx, roundNumber); err != nil {
		return fmt.Errorf("index clean-up failed: %w", err)
	}
	return nil
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
			if err := p.IndexBlock(ctx, b.Block, b.State); err != nil {
				p.log.Warn(fmt.Sprintf("indexing block %v failed", b.Block.GetRoundNumber()), logger.Error(err))
			}
		}
	}
}

// create - creates proof index DB entries
func (p *ProofIndexer) create(ctx context.Context, block *types.Block, stateReader UnitAndProof) (err error) {
	if block.GetRoundNumber() <= p.latestIndexedBlockNumber() {
		return fmt.Errorf("block %d already indexed", block.GetRoundNumber())
	}
	dbTx, err := p.storage.StartTx()
	if err != nil {
		return fmt.Errorf("start DB transaction failed: %w", err)
	}

	// commit if no error, rollback if any error
	defer func() {
		if err != nil {
			if e := dbTx.Rollback(); e != nil {
				err = errors.Join(err, fmt.Errorf("index transaction rollback failed: %w", e))
			}
		}
	}()
	defer func() {
		if err == nil {
			if e := dbTx.Commit(); e != nil {
				err = errors.Join(err, fmt.Errorf("index transaction commit failed: %w", e))
			}
		}
	}()

	var history historyIndex
	for i, tx := range block.Transactions {
		// write down tx index for generating block proofs
		txoHash := tx.TransactionOrder.Hash(p.hashAlgorithm)
		if err = dbTx.Write(txoHash, &TxIndex{
			RoundNumber:  block.GetRoundNumber(),
			TxOrderIndex: i,
		}); err != nil {
			return err
		}

		// generate and store unit proofs for all updated units
		txrHash := tx.Hash(p.hashAlgorithm)
		for _, unitID := range tx.ServerMetadata.TargetUnits {
			var unit *state.Unit
			unit, err = stateReader.GetUnit(unitID, true)
			if err != nil {
				return fmt.Errorf("unit load failed: %w", err)
			}
			unitLogs := unit.Logs()
			p.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("Generating %d proof(s) for unit %X", len(unitLogs), unitID))
			for j, unitLog := range unitLogs {
				if !bytes.Equal(unitLog.TxRecordHash, txrHash) {
					continue
				}
				usp, e := stateReader.CreateUnitStateProof(unitID, j)
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

	// commit if no error, rollback if any error
	defer func() {
		if err != nil {
			if e := dbTx.Rollback(); e != nil {
				err = errors.Join(err, fmt.Errorf("history clean rollback failed: %w", e))
			}
		}
	}()
	defer func() {
		if err == nil {
			if e := dbTx.Commit(); e != nil {
				err = errors.Join(err, fmt.Errorf("history clean commit failed: %w", e))
			}
		}
	}()

	for _, key := range history.UnitProofIndexKeys {
		if e := dbTx.Delete(key); e != nil {
			err = errors.Join(err, fmt.Errorf("unable to delete unit poof index: %w", e))
		}
	}
	p.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("Removed old unit proofs from round %d, index size %d", d, len(history.UnitProofIndexKeys)))
	return err
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
