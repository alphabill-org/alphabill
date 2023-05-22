package txsubmitter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type (
	TxSubmission struct {
		UnitID      wallet.UnitID
		TxHash      wallet.TxHash
		Transaction *txsystem.Transaction
		Proof       *wallet.Proof
	}

	TxSubmissionBatch struct {
		sender      wallet.PubKey
		submissions []*TxSubmission
		maxTimeout  uint64
		backend     BackendAPI
	}

	BackendAPI interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error
		GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	}
)

func (s *TxSubmission) ToBatch(backend BackendAPI, sender wallet.PubKey) *TxSubmissionBatch {
	return &TxSubmissionBatch{
		sender:      sender,
		backend:     backend,
		submissions: []*TxSubmission{s},
		maxTimeout:  s.Transaction.Timeout(),
	}
}

func (s *TxSubmission) Confirmed() bool {
	return s.Proof != nil
}

func NewBatch(sender wallet.PubKey, backend BackendAPI) *TxSubmissionBatch {
	return &TxSubmissionBatch{
		sender:  sender,
		backend: backend,
	}
}

func (t *TxSubmissionBatch) Add(sub *TxSubmission) {
	t.submissions = append(t.submissions, sub)
	if sub.Transaction.Timeout() > t.maxTimeout {
		t.maxTimeout = sub.Transaction.Timeout()
	}
}

func (t *TxSubmissionBatch) Submissions() []*TxSubmission {
	return t.submissions
}

func (t *TxSubmissionBatch) transactions() []*txsystem.Transaction {
	txs := make([]*txsystem.Transaction, 0, len(t.submissions))
	for _, sub := range t.submissions {
		txs = append(txs, sub.Transaction)
	}
	return txs
}

func (t *TxSubmissionBatch) SendTx(ctx context.Context, confirmTx bool) error {
	if len(t.submissions) == 0 {
		return errors.New("no transactions to send")
	}
	err := t.backend.PostTransactions(ctx, t.sender, &txsystem.Transactions{Transactions: t.transactions()})
	if err != nil {
		return err
	}
	if confirmTx {
		return t.confirmUnitsTx(ctx)
	}
	return nil
}

func (t *TxSubmissionBatch) confirmUnitsTx(ctx context.Context) error {
	log.Info("Confirming submitted transactions")

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("confirming transactions interrupted: %w", err)
		}

		roundNr, err := t.backend.GetRoundNumber(ctx)
		if err != nil {
			return err
		}
		if roundNr >= t.maxTimeout {
			log.Info(fmt.Sprintf("Tx confirmation timeout is reached, block (#%v)", roundNr))
			for _, sub := range t.submissions {
				if !sub.Confirmed() {
					log.Info(fmt.Sprintf("Tx not confirmed for UnitID=%X", sub.UnitID))
				}
			}
			return errors.New("confirmation timeout")
		}
		unconfirmed := false
		for _, sub := range t.submissions {
			if sub.Confirmed() || roundNr >= sub.Transaction.Timeout() {
				continue
			}
			proof, err := t.backend.GetTxProof(ctx, sub.UnitID, sub.TxHash)
			if err != nil {
				return err
			}
			if proof != nil {
				log.Debug(fmt.Sprintf("UnitID=%X is confirmed", sub.UnitID))
				sub.Proof = proof
			}
			unconfirmed = unconfirmed || !sub.Confirmed()
		}
		if unconfirmed {
			time.Sleep(500 * time.Millisecond)
		} else {
			log.Info("All transactions confirmed")
			return nil
		}
	}
}
