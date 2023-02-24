package tokens

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"sort"

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
		id     twb.UnitID
		txHash twb.TxHash
		tx     *txsystem.Transaction
	}

	txSubmissionBatch struct {
		sender      twb.PubKey
		submissions []*txSubmission
		submit      postTransactions
	}

	txPreprocessor func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error

	postTransactions func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error
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
	err = sub.toBatch(w.backend.PostTransactions, acc.PubKey).sendTx(ctx)
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
	var keyHash []byte
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	key, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}
	keyHash = key.PubKeyHash.Sha256
	attrs.SetBearer(bearerPredicateFromHash(keyHash))

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
	err = sub.toBatch(w.backend.PostTransactions, key.PubKey).sendTx(ctx)
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
	txSub := &txSubmission{
		id:     unitId,
		tx:     tx,
		txHash: gtx.Hash(crypto.SHA256),
	}
	return txSub, nil
}

func (s *txSubmission) toBatch(sendFunc postTransactions, sender twb.PubKey) *txSubmissionBatch {
	return &txSubmissionBatch{
		sender:      sender,
		submit:      sendFunc,
		submissions: []*txSubmission{s},
	}
}

func (t *txSubmissionBatch) add(sub *txSubmission) {
	t.submissions = append(t.submissions, sub)
}

func (t *txSubmissionBatch) transactions() []*txsystem.Transaction {
	txs := make([]*txsystem.Transaction, 0, len(t.submissions))
	for _, sub := range t.submissions {
		txs = append(txs, sub.tx)
	}
	return txs
}

func (t *txSubmissionBatch) sendTx(ctx context.Context) error {
	if len(t.submissions) == 0 {
		return errors.New("no transactions to send")
	}
	err := t.submit(ctx, t.sender, &txsystem.Transactions{Transactions: t.transactions()})
	if err != nil {
		return err
	}
	return nil
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

func bearerPredicateFromHash(receiverPubKeyHash []byte) ttxs.Predicate {
	if receiverPubKeyHash != nil {
		return script.PredicatePayToPublicKeyHashDefault(receiverPubKeyHash)
	}
	return script.PredicateAlwaysTrue()
}

func bearerPredicateFromPubKey(receiverPubKey twb.PubKey) ttxs.Predicate {
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
		sender: acc.PubKey,
		submit: w.backend.PostTransactions,
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
	return batch.sendTx(ctx)
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

func (w *Wallet) confirmUnitTx(ctx context.Context, sub *txSubmission, timeout uint64) error {
	submissions := make(map[string]*txSubmission, 1)
	submissions[sub.id.String()] = sub
	return w.confirmUnitsTx(ctx, submissions, timeout)
}

func (w *Wallet) confirmUnitsTx(ctx context.Context, subs map[string]*txSubmission, maxTimeout uint64) error {
	// ...or don't
	if !w.confirmTx {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, sub := range subs {
		log.Info(fmt.Sprintf("Waiting for UnitID=%X", sub.id))
		// TODO: Get token's tx proof from the backend (AB-755)
	}

	rn, err := w.getRoundNumber(ctx)
	if err != nil {
		return err
	}
	if rn >= maxTimeout {
		log.Info(fmt.Sprintf("Tx confirmation timeout is reached, block (#%v)", rn))
		for _, sub := range subs {
			log.Info(fmt.Sprintf("Tx not found for UnitID=%X", sub.id))
		}
		return errors.New("confirmation timeout")
	}

	panic("not implemented")
}
