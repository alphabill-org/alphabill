package money

import (
	"context"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
)

type (
	TxPublisher struct {
		wallet  *wallet.Wallet
		backend BackendAPI
	}
)

func NewTxPublisher(wallet *wallet.Wallet, backend BackendAPI) *TxPublisher {
	return &TxPublisher{
		wallet:  wallet,
		backend: backend,
	}
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *TxPublisher) SendTx(ctx context.Context, tx *types.TransactionOrder, _ []byte) (*types.TxProof, error) {
	roundNumber, err := w.backend.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	err = w.wallet.SendTransaction(ctx, tx, nil)
	if err != nil {
		return nil, err
	}
	txProofs, err := w.WaitForConfirmation(ctx, []*types.TransactionOrder{tx}, roundNumber, tx.Payload.ClientMetadata.Timeout)
	if err != nil {
		return nil, err
	}
	return txProofs[0], nil
}

func (w *TxPublisher) WaitForConfirmation(ctx context.Context, pendingTxs []*types.TransactionOrder, latestRoundNumber, timeout uint64) ([]*types.TxProof, error) {
	wlog.Info("waiting for confirmation(s)...")
	latestBlockNumber := latestRoundNumber
	txsLog := NewTxLog(pendingTxs)
	for latestBlockNumber <= timeout {
		blockBytes, err := w.wallet.AlphabillClient.GetBlock(ctx, latestBlockNumber)
		if err != nil {
			return nil, err
		}
		if blockBytes == nil || (len(blockBytes) == 1 && blockBytes[0] == 0xf6) { // 0xf6 cbor Null
			// block might be empty, check latest round number
			latestRoundNumber, err = w.wallet.AlphabillClient.GetRoundNumber(ctx)
			if err != nil {
				return nil, err
			}
			if latestRoundNumber > latestBlockNumber {
				latestBlockNumber++
				continue
			}
			// wait for some time before retrying to fetch new block
			select {
			case <-time.After(time.Second):
				continue
			case <-ctx.Done():
				return nil, nil
			}
		}
		block := &types.Block{}
		if err := cbor.Unmarshal(blockBytes, block); err != nil {
			return nil, fmt.Errorf("failed to unmarshal block: %w", err)
		}
		for txIdx, tx := range block.Transactions {
			if txsLog.Contains(tx) {
				wlog.Info("confirmed tx ", hexutil.Encode(tx.TransactionOrder.UnitID()))
				err = txsLog.RecordTx(tx, txIdx, block)
				if err != nil {
					return nil, err
				}
				if txsLog.IsAllTxsConfirmed() {
					wlog.Info("transaction(s) confirmed")
					return txsLog.GetAllRecordedProofs(), nil
				}
			}
		}
		latestBlockNumber++
	}
	return nil, ErrTxFailedToConfirm
}
