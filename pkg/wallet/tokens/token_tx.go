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

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	txSubmission struct {
		id        backend.UnitID
		txHash    backend.TxHash
		tx        *txsystem.Transaction
		confirmed bool
		txProof   *block.BlockProof
	}

	txSubmissionBatch struct {
		sender      backend.PubKey
		submissions []*txSubmission
		maxTimeout  uint64
		backend     TokenBackend
	}

	txPreprocessor func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error
)

func (w *Wallet) newType(ctx context.Context, accNr uint64, attrs AttrWithSubTypeCreationInputs, typeId backend.TokenTypeID, subtypePredicateArgs []*PredicateInput) (backend.TokenTypeID, error) {
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	err := w.ensureFeeCredit(ctx, accNr-1, 1)
	if err != nil {
		return nil, err
	}
	acc, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}
	sub, err := w.prepareTx(ctx, backend.UnitID(typeId), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
	return backend.TokenTypeID(sub.id), nil
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

func (w *Wallet) newToken(ctx context.Context, accNr uint64, attrs MintAttr, tokenId backend.TokenID, mintPredicateArgs []*PredicateInput) (backend.TokenID, error) {
	var keyHash []byte
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	err := w.ensureFeeCredit(ctx, accNr-1, 1)
	if err != nil {
		return nil, err
	}
	key, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}
	keyHash = key.PubKeyHash.Sha256
	attrs.SetBearer(bearerPredicateFromHash(keyHash))

	sub, err := w.prepareTx(ctx, backend.UnitID(tokenId), attrs, key, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
	return backend.TokenID(sub.id), nil
}

func RandomID() (backend.UnitID, error) {
	id := make([]byte, 32)
	_, err := rand.Read(id)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (w *Wallet) prepareTx(ctx context.Context, unitId backend.UnitID, attrs proto.Message, ac *account.AccountKey, txps txPreprocessor) (*txSubmission, error) {
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
	tx := createTx(w.systemID, unitId, roundNumber+txTimeoutRoundCount, ac.PrivKeyHash)
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
	gtx, err = ttxs.NewGenericTx(tx)
	if err != nil {
		return nil, err
	}
	// TODO should not rely on server metadata
	gtx.SetServerMetadata(&txsystem.ServerMetadata{Fee: 1})

	// convert again for hashing as the tx might have been modified
	txSub := &txSubmission{
		id:     unitId,
		tx:     tx,
		txHash: gtx.Hash(crypto.SHA256),
	}
	return txSub, nil
}

func (s *txSubmission) toBatch(backend TokenBackend, sender backend.PubKey) *txSubmissionBatch {
	return &txSubmissionBatch{
		sender:      sender,
		backend:     backend,
		submissions: []*txSubmission{s},
		maxTimeout:  s.tx.ClientMetadata.Timeout,
	}
}

func (t *txSubmissionBatch) add(sub *txSubmission) {
	t.submissions = append(t.submissions, sub)
	if sub.tx.ClientMetadata.Timeout > t.maxTimeout {
		t.maxTimeout = sub.tx.ClientMetadata.Timeout
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
			if sub.confirmed || roundNr >= sub.tx.ClientMetadata.Timeout {
				continue
			}
			proof, err := t.backend.GetTxProof(ctx, sub.id, sub.txHash)
			if err != nil {
				return err
			}
			if proof != nil {
				log.Debug(fmt.Sprintf("UnitID=%X is confirmed", sub.id))
				sub.confirmed = true
				sub.txProof = proof.Proof
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

func signTx(gtx txsystem.GenericTransaction, ac *account.AccountKey) (ttxs.Predicate, error) {
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

func newFungibleTransferTxAttrs(token *backend.TokenUnit, receiverPubKey []byte) *ttxs.TransferFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating transfer with bl=%X", token.TxHash))
	return &ttxs.TransferFungibleTokenAttributes{
		Type:                         token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		Value:                        token.Amount,
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

func newNonFungibleTransferTxAttrs(token *backend.TokenUnit, receiverPubKey []byte) *ttxs.TransferNonFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating NFT transfer with bl=%X", token.TxHash))
	return &ttxs.TransferNonFungibleTokenAttributes{
		NftType:                      token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

func bearerPredicateFromHash(receiverPubKeyHash []byte) ttxs.Predicate {
	if receiverPubKeyHash != nil {
		return script.PredicatePayToPublicKeyHashDefault(receiverPubKeyHash)
	}
	return script.PredicateAlwaysTrue()
}

func bearerPredicateFromPubKey(receiverPubKey backend.PubKey) ttxs.Predicate {
	var h []byte
	if receiverPubKey != nil {
		h = hash.Sum256(receiverPubKey)
	}
	return bearerPredicateFromHash(h)
}

func newSplitTxAttrs(token *backend.TokenUnit, amount uint64, receiverPubKey []byte) *ttxs.SplitFungibleTokenAttributes {
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

func newBurnTxAttrs(token *backend.TokenUnit, targetStateHash []byte) *ttxs.BurnFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating burn tx of unit=%X with bl=%X, new value=%v", token.ID, token.TxHash, token.Amount))
	return &ttxs.BurnFungibleTokenAttributes{
		Type:                         token.TypeID,
		Value:                        token.Amount,
		Nonce:                        targetStateHash,
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

// assumes there's sufficient balance for the given amount, sends transactions immediately
func (w *Wallet) doSendMultiple(ctx context.Context, amount uint64, tokens []*backend.TokenUnit, acc *account.AccountKey, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) error {
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

func (w *Wallet) prepareSplitOrTransferTx(ctx context.Context, acc *account.AccountKey, amount uint64, token *backend.TokenUnit, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (*txSubmission, error) {
	var attrs AttrWithInvariantPredicateInputs
	if amount >= token.Amount {
		attrs = newFungibleTransferTxAttrs(token, receiverPubKey)
	} else {
		attrs = newSplitTxAttrs(token, amount, receiverPubKey)
	}
	sub, err := w.prepareTx(ctx, backend.UnitID(token.ID), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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

func createTx(systemID []byte, unitId []byte, timeout uint64, fcrID []byte) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              systemID,
		UnitId:                unitId,
		TransactionAttributes: new(anypb.Any),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout:           timeout,
			MaxFee:            tx_builder.MaxFee,
			FeeCreditRecordId: fcrID,
		},
		// OwnerProof is added after whole transaction is built
	}
}
