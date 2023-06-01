package wallet

import (
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
)

const (
	defaultTxTimeout = 10
)

type (
	TxPublisher struct {
		vdClient    *VDClient
		txConverter *TxConverter
	}

	// A single use backend that records the round number before each send
	VDBackend struct {
		vdClient         *VDClient
		startRoundNumber uint64
		txTimeout        uint64
	}
)

func NewTxPublisher(vdClient *VDClient, txConverter *TxConverter) *TxPublisher {
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
	vdBackend := &VDBackend{
		vdClient: w.vdClient,
		txTimeout: tx.Timeout(),
	}
	txBatch := txSub.ToBatch(vdBackend, senderPubKey)
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

func (v *VDBackend) GetRoundNumber(ctx context.Context) (uint64, error) {
	return v.vdClient.GetRoundNumber(ctx)
}

func (v *VDBackend) PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
	currentRoundNumber, err := v.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	v.startRoundNumber = currentRoundNumber + 1
	if v.txTimeout == 0 {
		v.txTimeout = defaultTxTimeout
	}

	return v.vdClient.PostTransactions(ctx, pubKey, txs)
}

func (v *VDBackend) GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	return v.vdClient.GetTxProof(ctx, unitID, txHash, v.startRoundNumber, v.txTimeout)
}
