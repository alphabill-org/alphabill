package wallet

import (
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
)

const (
	defaultTxTimeout = 10
)

type (
	TxPublisher struct {
		vdClient *VDClient
	}

	// A single use backend that records the round number before each send
	VDBackend struct {
		vdClient         *VDClient
		startRoundNumber uint64
		txTimeout        uint64
	}
)

func NewTxPublisher(vdClient *VDClient) *TxPublisher {
	return &TxPublisher{
		vdClient: vdClient,
	}
}

func (w *TxPublisher) SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error) {
	vdBackend := &VDBackend{
		vdClient:  w.vdClient,
		txTimeout: tx.Timeout(),
	}
	txSub := &txsubmitter.TxSubmission{
		UnitID:      tx.UnitID(),
		TxHash:      tx.Hash(crypto.SHA256),
		Transaction: tx,
	}
	txBatch := txSub.ToBatch(vdBackend, senderPubKey)
	err := txBatch.SendTx(ctx, true)
	if err != nil {
		return nil, err
	}

	return txBatch.Submissions()[0].Proof, nil
}

func (w *TxPublisher) Close() {
	w.vdClient.Close()
}

func (v *VDBackend) GetRoundNumber(ctx context.Context) (uint64, error) {
	return v.vdClient.GetRoundNumber(ctx)
}

func (v *VDBackend) PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
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
