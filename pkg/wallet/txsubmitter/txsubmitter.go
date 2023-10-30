package txsubmitter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
	"github.com/alphabill-org/alphabill/pkg/wallet"
)

type (
	TxSubmission struct {
		UnitID      types.UnitID
		TxHash      wallet.TxHash
		Transaction *types.TransactionOrder
		Proof       *wallet.Proof
	}

	TxSubmissionBatch struct {
		sender      wallet.PubKey
		submissions []*TxSubmission
		maxTimeout  uint64
		backend     BackendAPI
		log         *slog.Logger
	}

	BackendAPI interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error
		GetTxProof(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	}
)

func (s *TxSubmission) ToBatch(backend BackendAPI, sender wallet.PubKey, log *slog.Logger) *TxSubmissionBatch {
	return &TxSubmissionBatch{
		sender:      sender,
		backend:     backend,
		submissions: []*TxSubmission{s},
		maxTimeout:  s.Transaction.Timeout(),
		log:         log,
	}
}

func (s *TxSubmission) Confirmed() bool {
	return s.Proof != nil
}

func NewBatch(sender wallet.PubKey, backend BackendAPI, log *slog.Logger) *TxSubmissionBatch {
	return &TxSubmissionBatch{
		sender:  sender,
		backend: backend,
		log:     log,
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

func (t *TxSubmissionBatch) transactions() []*types.TransactionOrder {
	txs := make([]*types.TransactionOrder, 0, len(t.submissions))
	for _, sub := range t.submissions {
		txs = append(txs, sub.Transaction)
	}
	return txs
}

func (t *TxSubmissionBatch) SendTx(ctx context.Context, confirmTx bool) error {
	if len(t.submissions) == 0 {
		return errors.New("no transactions to send")
	}
	err := t.backend.PostTransactions(ctx, t.sender, &wallet.Transactions{Transactions: t.transactions()})
	if err != nil {
		return err
	}
	if confirmTx {
		return t.confirmUnitsTx(ctx)
	}
	return nil
}

func (t *TxSubmissionBatch) confirmUnitsTx(ctx context.Context) error {
	t.log.InfoContext(ctx, "Confirming submitted transactions")

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("confirming transactions interrupted: %w", err)
		}

		roundNr, err := t.backend.GetRoundNumber(ctx)
		if err != nil {
			return err
		}
		unconfirmed := false
		for _, sub := range t.submissions {
			if sub.Confirmed() {
				continue
			}

			// Tx can be included in block sub.Transaction().Timeout()-1 at latest.
			// But let's wait until node is at round sub.Transaction().Timeout()+1
			// to give backend more time to fetch and process the block. A more reliable
			// solution would be to use the latest round number that backend has processed,
			// instead of the latest round number from node.
			if roundNr <= sub.Transaction.Timeout() {
				proof, err := t.backend.GetTxProof(ctx, sub.UnitID, sub.TxHash)
				if err != nil {
					return err
				}
				if proof != nil {
					t.log.DebugContext(ctx, "Unit is confirmed", logger.UnitID(sub.UnitID))
					sub.Proof = proof
				}
			}

			unconfirmed = unconfirmed || !sub.Confirmed()
		}
		if unconfirmed {
			// If this was the last attempt to get proofs, log the ones that timed out.
			if roundNr > t.maxTimeout {
				t.log.InfoContext(ctx, "Tx confirmation timeout is reached", logger.Round(roundNr))
				for _, sub := range t.submissions {
					if !sub.Confirmed() {
						t.log.InfoContext(ctx, fmt.Sprintf("Tx not confirmed for hash=%X", sub.TxHash), logger.UnitID(sub.UnitID))
					}
				}
				return errors.New("confirmation timeout")
			}

			time.Sleep(500 * time.Millisecond)
		} else {
			t.log.InfoContext(ctx, "All transactions confirmed")
			return nil
		}
	}
}
