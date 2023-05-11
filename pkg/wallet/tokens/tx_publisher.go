package tokens

import (
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
)

type (
	TxPublisher struct {
		backend     *client.TokenBackend
		txConverter TxConverter
	}

	TxConverter interface {
		ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)
	}
)

func NewTxPublisher(backendClient *client.TokenBackend, txConverter TxConverter) *TxPublisher {
	return &TxPublisher{
		backend:     backendClient,
		txConverter: txConverter,
	}
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *TxPublisher) SendTx(ctx context.Context, tx *txsystem.Transaction, senderPubKey []byte) (*block.TxProof, error) {
	gtx, err := w.txConverter.ConvertTx(tx)
	if err != nil {
		return nil, err
	}
	// TODO txhash should not rely on server metadata
	gtx.SetServerMetadata(&txsystem.ServerMetadata{Fee: 1})
	txSub := &txSubmission{
		id:     tx.UnitId,
		tx:     tx,
		txHash: gtx.Hash(crypto.SHA256),
	}
	txBatch := txSub.toBatch(w.backend, senderPubKey)
	err = txBatch.sendTx(ctx, true)
	if err != nil {
		return nil, err
	}
	return &block.TxProof{
		Tx:          tx,
		Proof:       txBatch.submissions[0].txProof,
		BlockNumber: txBatch.submissions[0].txProof.UnicityCertificate.GetRoundNumber(),
	}, nil
}
