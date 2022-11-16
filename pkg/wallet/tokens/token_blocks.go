package tokens

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type (
	BlockListener func(b *block.Block) error
)

func (l BlockListener) ProcessBlock(b *block.Block) error {
	return l(b)
}

func (w *Wallet) ProcessBlock(b *block.Block) error {
	if !bytes.Equal(tokens.DefaultTokenTxSystemIdentifier, b.GetSystemIdentifier()) {
		return ErrInvalidBlockSystemID
	}
	return w.db.WithTransaction(func(txc TokenTxContext) error {
		blockNumber := b.BlockNumber
		lastBlockNumber, err := txc.GetBlockNumber()
		if err != nil {
			return err
		}
		if blockNumber != lastBlockNumber+1 {
			return errors.New(fmt.Sprintf("invalid block height. Received blockNumber %d current wallet blockNumber %d", blockNumber, lastBlockNumber))
		}

		if len(b.Transactions) != 0 {
			log.Info("Processing non-empty block: ", b.BlockNumber)

			// lists tokens for all keys and with 'always true' predicate
			accounts, err := w.mw.GetAccountKeys()
			if err != nil {
				return err
			}
			for _, tx := range b.Transactions {
				for n := 0; n <= len(accounts); n++ {
					var keyHashes *wallet.KeyHashes
					if n > 0 {
						keyHashes = accounts[n-1].PubKeyHash
					}
					err = w.readTx(txc, tx, uint64(n), keyHashes)
					if err != nil {
						return err
					}
				}
				log.Info(fmt.Sprintf("tx with UnitID=%X", tx.UnitId))
			}
		}

		lst := w.blockListener
		if lst != nil {
			go func() {
				err := lst.ProcessBlock(b)
				if err != nil {
					log.Info(fmt.Sprintf("Failed to process a block #%v with blockListener", b.BlockNumber))
				}
			}()
		}

		return txc.SetBlockNumber(b.BlockNumber)
	})
}

func (w *Wallet) Sync(ctx context.Context) error {
	if !w.sync {
		return nil
	}
	latestBlockNumber, err := w.db.Do().GetBlockNumber()
	if err != nil {
		return err
	}
	log.Info("Synchronizing tokens from block #", latestBlockNumber)
	return w.mw.Wallet.SyncToMaxBlockNumber(ctx, latestBlockNumber)
}

func (w *Wallet) syncToUnit(ctx context.Context, id TokenID, timeout uint64) error {
	submissions := make(map[string]*submittedTx, 1)
	submissions[id.String()] = &submittedTx{id, timeout}
	return w.syncToUnits(ctx, submissions, timeout)
}

func (w *Wallet) syncToUnits(ctx context.Context, subs map[string]*submittedTx, maxTimeout uint64) error {
	// ...or don't
	if !w.sync {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)

	log.Info("Waiting the transactions to be finalized")
	var bl BlockListener = func(b *block.Block) error {
		log.Debug(fmt.Sprintf("Listener has got the block #%v", b.BlockNumber))
		if b.BlockNumber > maxTimeout {
			log.Info(fmt.Sprintf("Sync timeout is reached, block (#%v)", b.BlockNumber))
			for _, sub := range subs {
				log.Info(fmt.Sprintf("Tx not found for UnitID=%X", sub.id))
			}
			cancel()
		}
		for _, tx := range b.Transactions {
			id := TokenID(tx.UnitId).String()
			if sub, found := subs[id]; found {
				log.Info(fmt.Sprintf("Tx with UnitID=%X is in the block #%v", sub.id, b.BlockNumber))
				delete(subs, id)
			}
			if len(subs) == 0 {
				cancel()
			}
		}
		return nil
	}
	w.blockListener = bl

	defer func() {
		w.blockListener = nil
		cancel()
	}()

	return w.syncUntilCanceled(ctx)
}

func (w *Wallet) syncUntilCanceled(ctx context.Context) error {
	latestBlockNumber, err := w.db.Do().GetBlockNumber()
	if err != nil {
		return err
	}
	log.Info("Synchronizing tokens from block #", latestBlockNumber)
	return w.mw.Wallet.Sync(ctx, latestBlockNumber)
}
