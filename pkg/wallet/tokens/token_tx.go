package tokens

import (
	"context"
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	txSubmission struct {
		id        twb.UnitID
		txHash    twb.TxHash
		tx        *txsystem.Transaction
		confirmed bool
	}

	txSubmissionBatch struct {
		sender      twb.PubKey
		submissions []*txSubmission
		maxTimeout  uint64
		backend     TokenBackend
	}

	txPreprocessor func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error
)

func (w *Wallet) newType(ctx context.Context, accNr uint64, attrs AttrWithSubTypeCreationInputs, typeId twb.TokenTypeID, subtypePredicateArgs []*PredicateInput) (twb.TokenTypeID, error) {
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	acc, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}
	sub, err := w.prepareTx(ctx, twb.UnitID(typeId), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.am, subtypePredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.SetSubTypeCreationPredicateSignatures(signatures)
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return nil, err
	}
	err = sub.toBatch(w.backend, acc.PubKey).sendTx(ctx, w.confirmTx)
	if err != nil {
		return nil, err
	}
	return twb.TokenTypeID(sub.id), nil
}

func preparePredicateSignatures(am account.Manager, args []*PredicateInput, gtx txsystem.GenericTransaction) ([][]byte, error) {
	signatures := make([][]byte, 0, len(args))
	for _, input := range args {
		if len(input.Argument) > 0 {
			signatures = append(signatures, input.Argument)
		} else if input.AccountNumber > 0 {
			ac, err := am.GetAccountKey(input.AccountNumber - 1)
			if err != nil {
				return nil, err
			}
			sig, err := signTx(gtx, ac)
			if err != nil {
				return nil, err
			}
			signatures = append(signatures, sig)
		} else {
			return nil, fmt.Errorf("invalid account for creation input: %v", input.AccountNumber)
		}
	}
	return signatures, nil
}

func (w *Wallet) newToken(ctx context.Context, accNr uint64, attrs MintAttr, tokenId twb.TokenID, mintPredicateArgs []*PredicateInput) (twb.TokenID, error) {
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	key, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}

	sub, err := w.prepareTx(ctx, twb.UnitID(tokenId), attrs, key, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.am, mintPredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.SetTokenCreationPredicateSignatures(signatures)
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return nil, err
	}
	err = sub.toBatch(w.backend, key.PubKey).sendTx(ctx, w.confirmTx)
	if err != nil {
		return nil, err
	}
	return twb.TokenID(sub.id), nil
}

func RandomID() (twb.UnitID, error) {
	id := make([]byte, 32)
	_, err := rand.Read(id)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (w *Wallet) prepareTx(ctx context.Context, unitId twb.UnitID, attrs proto.Message, ac *account.AccountKey, txps txPreprocessor) (*txSubmission, error) {
	var err error
	if unitId == nil {
		unitId, err = RandomID()
		if err != nil {
			return nil, err
		}
	}
	log.Info(fmt.Sprintf("Preparing to send token tx, UnitID=%X, attributes: %v", unitId, reflect.TypeOf(attrs)))

	roundNumber, err := w.getRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	tx := createTx(w.systemID, unitId, roundNumber+txTimeoutRoundCount)
	err = anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	gtx, err := ttxs.NewGenericTx(tx)
	if err != nil {
		return nil, err
	}
	if txps != nil {
		// set fields before tx is signed
		err = txps(tx, gtx)
		if err != nil {
			return nil, err
		}
	}
	sig, err := signTx(gtx, ac)
	if err != nil {
		return nil, err
	}
	tx.OwnerProof = sig
	// convert again for hashing as the tx might have been modified
	gtx, err = ttxs.NewGenericTx(tx)
	if err != nil {
		return nil, err
	}
	txSub := &txSubmission{
		id:     unitId,
		tx:     tx,
		txHash: gtx.Hash(crypto.SHA256),
	}
	return txSub, nil
}

func (s *txSubmission) toBatch(backend TokenBackend, sender twb.PubKey) *txSubmissionBatch {
	return &txSubmissionBatch{
		sender:      sender,
		backend:     backend,
		submissions: []*txSubmission{s},
		maxTimeout:  s.tx.Timeout,
	}
}

func (t *txSubmissionBatch) add(sub *txSubmission) {
	t.submissions = append(t.submissions, sub)
	if sub.tx.Timeout > t.maxTimeout {
		t.maxTimeout = sub.tx.Timeout
	}
}

func (t *txSubmissionBatch) transactions() []*txsystem.Transaction {
	txs := make([]*txsystem.Transaction, 0, len(t.submissions))
	for _, sub := range t.submissions {
		txs = append(txs, sub.tx)
	}
	return txs
}

func (t *txSubmissionBatch) sendTx(ctx context.Context, confirmTx bool) error {
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

func (t *txSubmissionBatch) confirmUnitsTx(ctx context.Context) error {
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
				if !sub.confirmed {
					log.Info(fmt.Sprintf("Tx not confirmed for UnitID=%X", sub.id))
				}
			}
			return errors.New("confirmation timeout")
		}
		unconfirmed := false
		for _, sub := range t.submissions {
			if sub.confirmed || roundNr >= sub.tx.Timeout {
				continue
			}
			proof, err := t.backend.GetTxProof(ctx, sub.id, sub.txHash)
			if err != nil {
				return err
			}
			if proof != nil {
				log.Debug(fmt.Sprintf("UnitID=%X is confirmed", sub.id))
				sub.confirmed = true
			}
			unconfirmed = unconfirmed || !sub.confirmed
		}
		if unconfirmed {
			time.Sleep(500 * time.Millisecond)
		} else {
			log.Info("All transactions confirmed")
			return nil
		}
	}
}

func signTx(gtx txsystem.GenericTransaction, ac *account.AccountKey) (twb.Predicate, error) {
	if ac == nil {
		return script.PredicateArgumentEmpty(), nil
	}
	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(gtx.SigBytes())
	if err != nil {
		return nil, err
	}
	return script.PredicateArgumentPayToPublicKeyHashDefault(sig, ac.PubKey), nil
}

func newFungibleTransferTxAttrs(token *twb.TokenUnit, receiverPubKey []byte) *ttxs.TransferFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating transfer with bl=%X", token.TxHash))
	return &ttxs.TransferFungibleTokenAttributes{
		Type:                         token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		Value:                        token.Amount,
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

func newNonFungibleTransferTxAttrs(token *twb.TokenUnit, receiverPubKey []byte) *ttxs.TransferNonFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating NFT transfer with bl=%X", token.TxHash))
	return &ttxs.TransferNonFungibleTokenAttributes{
		NftType:                      token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

func bearerPredicateFromHash(receiverPubKeyHash []byte) twb.Predicate {
	if receiverPubKeyHash != nil {
		return script.PredicatePayToPublicKeyHashDefault(receiverPubKeyHash)
	}
	return script.PredicateAlwaysTrue()
}

func bearerPredicateFromPubKey(receiverPubKey twb.PubKey) twb.Predicate {
	var h []byte
	if receiverPubKey != nil {
		h = hash.Sum256(receiverPubKey)
	}
	return bearerPredicateFromHash(h)
}

func newSplitTxAttrs(token *twb.TokenUnit, amount uint64, receiverPubKey []byte) *ttxs.SplitFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating split with bl=%X, new value=%v", token.TxHash, amount))
	return &ttxs.SplitFungibleTokenAttributes{
		Type:                         token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		TargetValue:                  amount,
		RemainingValue:               token.Amount - amount,
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
}

// assumes there's sufficient balance for the given amount, sends transactions immediately
func (w *Wallet) doSendMultiple(ctx context.Context, amount uint64, tokens []*twb.TokenUnit, acc *account.AccountKey, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) error {
	var accumulatedSum uint64
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Amount > tokens[j].Amount
	})

	batch := &txSubmissionBatch{
		sender:  acc.PubKey,
		backend: w.backend,
	}
	for _, t := range tokens {
		remainingAmount := amount - accumulatedSum
		sub, err := w.prepareSplitOrTransferTx(ctx, acc, remainingAmount, t, receiverPubKey, invariantPredicateArgs)
		if err != nil {
			return err
		}
		batch.add(sub)
		accumulatedSum += t.Amount
		if accumulatedSum >= amount {
			break
		}
	}
	return batch.sendTx(ctx, w.confirmTx)
}

func (w *Wallet) prepareSplitOrTransferTx(ctx context.Context, acc *account.AccountKey, amount uint64, token *twb.TokenUnit, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (*txSubmission, error) {
	var attrs AttrWithInvariantPredicateInputs
	if amount >= token.Amount {
		attrs = newFungibleTransferTxAttrs(token, receiverPubKey)
	} else {
		attrs = newSplitTxAttrs(token, amount, receiverPubKey)
	}
	sub, err := w.prepareTx(ctx, twb.UnitID(token.ID), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.am, invariantPredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.SetInvariantPredicateSignatures(signatures)
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func createTx(systemID []byte, unitId []byte, timeout uint64) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              systemID,
		UnitId:                unitId,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		// OwnerProof is added after whole transaction is built
	}
}
