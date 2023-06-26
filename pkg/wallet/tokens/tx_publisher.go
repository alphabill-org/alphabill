package tokens

import (
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
)

type (
	TxPublisher struct {
		backend *client.TokenBackend
	}
)

func NewTxPublisher(backendClient *client.TokenBackend) *TxPublisher {
	return &TxPublisher{
		backend: backendClient,
	}
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *TxPublisher) SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error) {
	txSub := &txsubmitter.TxSubmission{
		UnitID:      tx.UnitID(),
		Transaction: tx,
		TxOrderHash: tx.Hash(crypto.SHA256),
	}
	txBatch := txSub.ToBatch(w.backend, senderPubKey)
	err := txBatch.SendTx(ctx, true)
	if err != nil {
		return nil, err
	}
	return txBatch.Submissions()[0].Proof, nil
}
