package state

import (
	"bytes"
	"crypto"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	keyLatestRoundNumber = []byte("latestRoundNumber")
)

type BlockAndState struct {
	Block *types.Block
	State *State
}

type UnitDataAndProof struct {
	_ struct{} `cbor:",toarray"`

	Data  UnitData
	Proof *types.UnitStateProof
}

type ProofIndexer struct {
	unitProofStorage keyvaluedb.KeyValueDB
	historySize      uint64 // number of rounds for which the history of unit states is kept
	log              *slog.Logger
}

func NewProofIndexer(unitProofStorage keyvaluedb.KeyValueDB, historySize uint64, l *slog.Logger) *ProofIndexer {
	return &ProofIndexer{
		unitProofStorage: unitProofStorage,
		historySize:      historySize,
		log:              l,
	}
}

func (p *ProofIndexer) Handle(e *event.Event) {
	switch e.EventType {
	case event.BlockFinalized:
		bas, ok := e.Content.(*struct {
			Block *types.Block
			State *State
		})
		if !ok {
			p.log.Warn(fmt.Sprintf("Invalid BlockFinalized event data. Expected %T got %T", &struct {
				Block *types.Block
				State *State
			}{}, e.Content))
			return
		}
		if err := p.create(&BlockAndState{Block: bas.Block, State: bas.State}); err != nil {
			p.log.Warn(fmt.Sprintf("Unable to index unit proofs for block %d: %v", bas.Block.GetRoundNumber(), err))
		}
	}
}

func (p *ProofIndexer) create(bas *BlockAndState) error {
	block := bas.Block
	if block.GetRoundNumber() < p.latestIndexedBlockNumber() {
		p.log.Debug(fmt.Sprintf("Block %d already indexed", block.GetRoundNumber()))
		return nil
	}
	p.log.Debug(fmt.Sprintf("Starting to generate unit proofs for block %d", bas.Block.GetRoundNumber()))
	dbtx, err := p.unitProofStorage.StartTx()
	if err != nil {
		return fmt.Errorf("unable to start DB transaction: %w", err)
	}

	defer func() {
		if err != nil {
			if e := dbtx.Rollback(); e != nil {
				p.log.Warn(fmt.Sprintf("unable to rollback unit proof index transaction: %v", err))
				return
			}
		}
		err = dbtx.Commit()
	}()

	uc := block.UnicityCertificate
	var handledTxOrders [][]byte
	for _, transaction := range block.Transactions {
		// TODO hash alg
		trHash := transaction.Hash(crypto.SHA256)
		targets := transaction.ServerMetadata.TargetUnits
		for _, id := range targets {
			u, err := bas.State.GetUnit(id, true)
			if err != nil {
				return fmt.Errorf("unable to load unit: %w", err)
			}
			logs := u.Logs()
			// todo: was trace
			p.log.Debug(fmt.Sprintf("Generating %d proof(s) for unit %X", len(logs), id))
			for i, l := range logs {
				if !bytes.Equal(l.txRecordHash, trHash) {
					continue
				}
				usp, err := bas.State.CreateUnitStateProof(id, i, uc)
				if err != nil {
					return fmt.Errorf("unable to create unit proof: %w", err)
				}
				// TODO hash alg
				key := bytes.Join([][]byte{id, transaction.TransactionOrder.Hash(crypto.SHA256)}, nil)
				handledTxOrders = append(handledTxOrders, key)
				if err = dbtx.Write(key, &UnitDataAndProof{
					Data:  l.newUnitData,
					Proof: usp,
				}); err != nil {
					return fmt.Errorf("unable to write unit proof: %w", err)
				}
			}
		}
	}
	// update latest round number
	roundNumber := block.GetRoundNumber()
	if err = dbtx.Write(keyLatestRoundNumber, roundNumber); err != nil {
		return fmt.Errorf("unable to write unit proof: %w", err)
	}

	// write delete index
	if err = dbtx.Write(util.Uint64ToBytes(roundNumber), handledTxOrders); err != nil {
		return fmt.Errorf("unable to write unit proof: %w", err)
	}

	if p.historySize > 0 && roundNumber > p.historySize {
		// remove old history
		d := roundNumber - p.historySize
		var indexToDelete [][]byte
		found, err := dbtx.Read(util.Uint64ToBytes(d), &indexToDelete)
		if err != nil {
			return fmt.Errorf("unable to read delete index: %w", err)
		}
		if !found {
			return nil
		}
		for _, key := range indexToDelete {
			if err := dbtx.Delete(key); err != nil {
				return fmt.Errorf("unable to delete index: %w", err)
			}
		}
		p.log.Debug(fmt.Sprintf("Removed old proofs from block %d, index size %d", d, len(indexToDelete)))
	}

	p.log.Debug(fmt.Sprintf("Done generating proofs for block %d", roundNumber))
	return nil
}

func (p *ProofIndexer) latestIndexedBlockNumber() uint64 {
	it := p.unitProofStorage.Find(keyLatestRoundNumber)
	defer func() { _ = it.Close() }()
	if !it.Valid() {
		return 0
	}
	var blockNr uint64
	if err := it.Value(&blockNr); err != nil {
		return 0
	}
	return blockNr
}
