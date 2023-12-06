package money

import (
	"context"
	"crypto"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wallet"
	"github.com/alphabill-org/alphabill/wallet/txsubmitter"
)

type TxPublisher struct {
	backend BackendAPI
	log     *slog.Logger
}

func NewTxPublisher(backend BackendAPI, log *slog.Logger) *TxPublisher {
	return &TxPublisher{
		backend: backend,
		log:     log,
	}
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *TxPublisher) SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error) {
	txSub := &txsubmitter.TxSubmission{
		UnitID:      tx.UnitID(),
		TxHash:      tx.Hash(crypto.SHA256),
		Transaction: tx,
	}
	w.log.InfoContext(ctx, fmt.Sprintf("Sending tx '%s' with hash: '%X'", tx.PayloadType(), tx.Hash(crypto.SHA256)), logger.UnitID(tx.UnitID()))
	txBatch := txSub.ToBatch(w.backend, senderPubKey, w.log)
	err := txBatch.SendTx(ctx, true)
	if err != nil {
		return nil, err
	}
	return txBatch.Submissions()[0].Proof, nil
}

func (w *TxPublisher) Close() {
}
