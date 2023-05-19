package wallet

import (
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/alphabill-org/alphabill/pkg/wallet/vd/client"
)

type (
	TxPublisher struct {
		vdClient    *client.VDClient
		txConverter *TxConverter
	}
)

func NewTxPublisher(vdClient *client.VDClient, txConverter *TxConverter) *TxPublisher {
	return &TxPublisher{
		vdClient: vdClient,
		txConverter: txConverter,
	}
}

func (w *TxPublisher) SendTx(ctx context.Context, tx *txsystem.Transaction, senderPubKey []byte) (*block.TxProof, error) {
	gtx, err := w.txConverter.ConvertTx(tx)
	if err != nil {
		return nil, err
	}
	// TODO txhash should not rely on server metadata
	gtx.SetServerMetadata(&txsystem.ServerMetadata{Fee: 1})
	txSub := &txsubmitter.TxSubmission{
		UnitID:      tx.UnitId,
		Transaction: tx,
		TxHash:      gtx.Hash(crypto.SHA256),
	}
	txBatch := txSub.ToBatch(w.vdClient, senderPubKey)
	err = txBatch.SendTx(ctx, true)
	if err != nil {
		return nil, err
	}
	return &block.TxProof{
		Tx:          tx,
		Proof:       txBatch.Submissions()[0].Proof.Proof,
		BlockNumber: txBatch.Submissions()[0].Proof.Proof.UnicityCertificate.GetRoundNumber(),
	}, nil
}

func (w *TxPublisher) Close() {
	w.vdClient.Close()
}
