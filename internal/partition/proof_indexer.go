package partition

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

var (
	IndexNotFound        = errors.New("index not found")
	keyLatestRoundNumber = []byte("latestRoundNumber")
)

type (
	BlockAndState struct {
		Block *types.Block
		State txsystem.UnitAndProof
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

func (p *ProofIndexer) Handle(block *types.Block, state txsystem.UnitAndProof) {
	newEvent := &BlockAndState{
		Block: block,
		State: state,
	}
	p.blockCh <- newEvent
}

func (p *ProofIndexer) GetDB() keyvaluedb.KeyValueDB {
	return p.storage
}

func (p *ProofIndexer) Close() {
	close(p.blockCh)
}

func (p *ProofIndexer) loop(ctx context.Context) {
	for b := range p.blockCh {
		roundNumber := b.Block.GetRoundNumber()
		p.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("indexing block %v", roundNumber))
		if err := p.create(ctx, b); err != nil {
			p.log.Warn(fmt.Sprintf("indexing block %v failed", roundNumber), logger.Error(err))
		}
		// clean-up
		if err := p.historyCleanup(ctx, roundNumber); err != nil {
			p.log.Warn(fmt.Sprintf("index clean-up failed"), logger.Error(err))
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

	uc := block.UnicityCertificate
	var history historyIndex
	for i, transaction := range block.Transactions {
		// write down tx index for generating block proofs
		oderHash := transaction.TransactionOrder.Hash(p.hashAlgorithm)
		if err = dbTx.Write(oderHash, &TxIndex{
			RoundNumber:  bas.Block.GetRoundNumber(),
			TxOrderIndex: i,
		}); err != nil {
			return err
		}
		history.TxIndexKey = oderHash
		// generate and store proofs for all updated units
		trHash := transaction.Hash(p.hashAlgorithm)
		targets := transaction.ServerMetadata.TargetUnits
		for _, id := range targets {
			var u *state.Unit
			u, err = bas.State.GetUnit(id, true)
			if err != nil {
				return fmt.Errorf("unit load failed: %w", err)
			}
			logs := u.Logs()
			p.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("Generating %d proof(s) for unit %X", len(logs), id))
			for i, l := range logs {
				if !bytes.Equal(l.TxRecordHash, trHash) {
					continue
				}
				usp, e := bas.State.CreateUnitStateProof(id, i, uc)
				if e != nil {
					err = errors.Join(err, fmt.Errorf("unit %X proof creatioon failed: %w", id, e))
					continue
				}
				res, e := state.MarshalUnitData(l.NewUnitData)
				if e != nil {
					err = errors.Join(err, fmt.Errorf("unit %X data encode failed: %w", id, e))
					continue
				}
				key := bytes.Join([][]byte{id, oderHash}, nil)
				history.UnitProofIndexKeys = append(history.UnitProofIndexKeys, key)
				if err = dbTx.Write(key, &types.UnitDataAndProof{
					UnitData: &types.StateUnitData{Data: res, Bearer: l.NewBearer},
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
	if err = dbTx.Write(util.Uint64ToBytes(roundNumber), history); err != nil {
		return fmt.Errorf("history index write failed: %w", err)
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
				err = errors.Join(fmt.Errorf("history clean rollback failed: %w", e))
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

func ReadTransactionIndex(db keyvaluedb.KeyValueDB, txOrderHash []byte) (*TxIndex, error) {
	index := &TxIndex{}
	f, err := db.Read(txOrderHash, index)
	if err != nil {
		return nil, fmt.Errorf("tx index query failed: %w", err)
	}
	if !f {
		return nil, IndexNotFound
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
		return nil, IndexNotFound
	}
	return index, nil
}
