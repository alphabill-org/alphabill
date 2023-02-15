package legacywallet

import (
	"bytes"
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type (
	BlockListener func(b *block.Block) error
)

func (l BlockListener) ProcessBlock(b *block.Block) error {
	return l(b)
}

func (w *Wallet) ProcessBlock(b *block.Block) error {
	if !bytes.Equal(w.SystemID(), b.GetSystemIdentifier()) {
		return ErrInvalidBlockSystemID
	}
	return w.db.WithTransaction(func(txc TokenTxContext) error {
		blockNumber := b.UnicityCertificate.InputRecord.RoundNumber
		lastBlockNumber, err := txc.GetBlockNumber()
		if err != nil {
			return err
		}
		// TODO: AB-505 block numbers are not sequential any more, gaps might appear as empty block are not stored and sent
		if lastBlockNumber >= blockNumber {
			return fmt.Errorf("invalid block number. Received blockNumber %d current wallet blockNumber %d", blockNumber, lastBlockNumber)
		}

		if len(b.Transactions) != 0 {
			log.Info("Processing non-empty block: ", b.UnicityCertificate.InputRecord.RoundNumber)

			// lists tokens for all keys and with 'always true' predicate
			accounts, err := w.am.GetAccountKeys()
			if err != nil {
				return err
			}
			for _, tx := range b.Transactions {
				for n := 0; n <= len(accounts); n++ {
					var keyHashes *account.KeyHashes
					if n > 0 {
						keyHashes = accounts[n-1].PubKeyHash
					}
					err = w.readTx(txc, tx, b, uint64(n), keyHashes)
					if err != nil {
						return err
					}
				}
				log.Info(fmt.Sprintf("tx with UnitID=%X", tx.UnitId))
			}
		}

		lst := w.blockListener
		if lst != nil {
			err := lst.ProcessBlock(b)
			if err != nil {
				log.Info(fmt.Sprintf("Failed to process a block #%v with blockListener", b.UnicityCertificate.InputRecord.RoundNumber))
				return err
			}
		}

		return txc.SetBlockNumber(b.UnicityCertificate.InputRecord.RoundNumber)
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
	return w.sdk.SyncToMaxBlockNumber(ctx, latestBlockNumber)
}

func (w *Wallet) syncToUnit(ctx context.Context, sub *submittedTx) error {
	return w.syncToUnits(ctx, newSubmissionSet().add(sub))
}

func (w *Wallet) syncToUnits(ctx context.Context, subs *submissionSet) error {
	// ...or don't
	if !w.sync {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)

	log.Info("Waiting the transactions to be finalized")
	var bl BlockListener = func(b *block.Block) error {
		log.Debug(fmt.Sprintf("Listener has got the block #%v", b.UnicityCertificate.InputRecord.RoundNumber))
		for _, tx := range b.Transactions {
			id := TokenID(tx.UnitId).String()
			if sub, found := subs.submissions[id]; found {
				log.Info(fmt.Sprintf("Tx with UnitID=%X is in the block #%v", sub.id, b.UnicityCertificate.InputRecord.RoundNumber))
				sub.confirm()
			}
		}
		confirmed := subs.confirmed()
		if confirmed {
			cancel()
		}
		if b.UnicityCertificate.InputRecord.RoundNumber >= subs.maxTimeout {
			log.Info(fmt.Sprintf("Sync timeout is reached, block (#%v)", b.UnicityCertificate.InputRecord.RoundNumber))
			for _, sub := range subs.submissions {
				if !sub.confirmed {
					log.Info(fmt.Sprintf("Tx not found for UnitID=%X", sub.id))
				}
			}
			cancel()

			if !confirmed {
				return errors.Errorf("did not confirm all transactions, timeout reached")
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
	return w.sdk.Sync(ctx, latestBlockNumber)
}
