package money

import (
	"context"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type (
	TxPublisher struct {
		wallet      *wallet.Wallet
		backend     BackendAPI
		txConverter *tx_builder.TxConverter
	}
)

func NewTxPublisher(wallet *wallet.Wallet, backend BackendAPI, txConverter *tx_builder.TxConverter) *TxPublisher {
	return &TxPublisher{
		wallet:      wallet,
		backend:     backend,
		txConverter: txConverter,
	}
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *TxPublisher) SendTx(ctx context.Context, tx *txsystem.Transaction, _ []byte) (*block.TxProof, error) {
	roundNumber, err := w.backend.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	err = w.wallet.SendTransaction(ctx, tx, nil)
	if err != nil {
		return nil, err
	}
	txProofs, err := w.WaitForConfirmation(ctx, []*txsystem.Transaction{tx}, roundNumber, tx.ClientMetadata.Timeout)
	if err != nil {
		return nil, err
	}
	return txProofs[0], nil
}

func (w *TxPublisher) WaitForConfirmation(ctx context.Context, pendingTxs []*txsystem.Transaction, latestRoundNumber, timeout uint64) ([]*block.TxProof, error) {
	wlog.Info("waiting for confirmation(s)...")
	latestBlockNumber := latestRoundNumber
	txsLog := NewTxLog(pendingTxs)
	for latestBlockNumber <= timeout {
		b, err := w.wallet.AlphabillClient.GetBlock(ctx, latestBlockNumber)
		if err != nil {
			return nil, err
		}
		if b == nil {
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

		// TODO no need to convert to generic tx?
		genericBlock, err := b.ToGenericBlock(w.txConverter)
		if err != nil {
			return nil, err
		}
		for _, gtx := range genericBlock.Transactions {
			tx := gtx.ToProtoBuf()
			if txsLog.Contains(tx) {
				wlog.Info("confirmed tx ", hexutil.Encode(tx.UnitId))
				err = txsLog.RecordTx(gtx, genericBlock)
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
