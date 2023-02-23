package tokens

import (
	"context"
	"crypto"
	"fmt"
	"math/rand"
	"reflect"
	"sort"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
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
	submittedTx struct {
		id      twb.TokenID
		txHash  []byte
		timeout uint64
	}

	txPreprocessor func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error
)

func (w *Wallet) newType(ctx context.Context, accNr uint64, attrs AttrWithSubTypeCreationInputs, typeId twb.TokenTypeID, subtypePredicateArgs []*PredicateInput) (twb.TokenID, error) {
	if accNr < 1 {
		return nil, errors.Errorf("invalid account number: %d", accNr)
	}
	acc, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}
	sub, err := w.sendTx(ctx, twb.TokenID(typeId), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
	return sub.id, nil
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
			return nil, errors.Errorf("invalid account for creation input: %v", input.AccountNumber)
		}
	}
	return signatures, nil
}

func (w *Wallet) newToken(ctx context.Context, accNr uint64, attrs MintAttr, tokenId twb.TokenID, mintPredicateArgs []*PredicateInput) (twb.TokenID, error) {
	var keyHash []byte
	if accNr < 1 {
		return nil, errors.Errorf("invalid account number: %d", accNr)
	}
	key, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}
	keyHash = key.PubKeyHash.Sha256
	attrs.SetBearer(bearerPredicateFromHash(keyHash))

	sub, err := w.sendTx(ctx, tokenId, attrs, key, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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

	return sub.id, nil
}

func RandomID() (twb.TokenID, error) {
	id := make([]byte, 32)
	_, err := rand.Read(id)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (w *Wallet) sendTx(ctx context.Context, unitId twb.TokenID, attrs proto.Message, ac *account.AccountKey, txps txPreprocessor) (*submittedTx, error) {
	txSub := &submittedTx{id: unitId}
	if unitId == nil {
		id, err := RandomID()
		if err != nil {
			return txSub, err
		}
		txSub.id = id
	}
	log.Info(fmt.Sprintf("Sending token tx, UnitID=%X, attributes: %v", txSub.id, reflect.TypeOf(attrs)))

	roundNumber, err := w.getRoundNumber(ctx)
	if err != nil {
		return txSub, err
	}
	tx := createTx(w.systemID, txSub.id, roundNumber+txTimeoutRoundCount)
	err = anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	if err != nil {
		return txSub, err
	}
	gtx, err := ttxs.NewGenericTx(tx)
	if err != nil {
		return txSub, err
	}
	if txps != nil {
		// set fields before tx is signed
		err = txps(tx, gtx)
		if err != nil {
			return txSub, err
		}
	}
	sig, err := signTx(gtx, ac)
	if err != nil {
		return txSub, err
	}
	tx.OwnerProof = sig
	err = w.backend.PostTransactions(ctx, ac.PubKey, &txsystem.Transactions{Transactions: []*txsystem.Transaction{tx}})
	if err != nil {
		return txSub, err
	}
	txSub.timeout = tx.Timeout
	txSub.txHash = gtx.Hash(crypto.SHA256)
	return txSub, nil
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
func (w *Wallet) doSendMultiple(ctx context.Context, amount uint64, tokens []*twb.TokenUnit, acc *account.AccountKey, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (map[string]*submittedTx, uint64, error) {
	var accumulatedSum uint64
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Amount > tokens[j].Amount
	})
	var maxTimeout uint64 = 0
	submissions := make(map[string]*submittedTx, 2)
	for _, t := range tokens {
		remainingAmount := amount - accumulatedSum
		// TODO: prepare transactions and then send them all at once (AB-754)
		sub, err := w.sendSplitOrTransferTx(ctx, acc, remainingAmount, t, receiverPubKey, invariantPredicateArgs)
		if sub.timeout > maxTimeout {
			maxTimeout = sub.timeout
		}
		if err != nil {
			return submissions, maxTimeout, err
		}
		submissions[sub.id.String()] = sub
		accumulatedSum += t.Amount
		if accumulatedSum >= amount {
			break
		}
	}
	return submissions, maxTimeout, nil
}

func (w *Wallet) sendSplitOrTransferTx(ctx context.Context, acc *account.AccountKey, amount uint64, token *twb.TokenUnit, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (*submittedTx, error) {
	var attrs AttrWithInvariantPredicateInputs
	if amount >= token.Amount {
		attrs = newFungibleTransferTxAttrs(token, receiverPubKey)
	} else {
		attrs = newSplitTxAttrs(token, amount, receiverPubKey)
	}
	sub, err := w.sendTx(ctx, token.ID, attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.am, invariantPredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.SetInvariantPredicateSignatures(signatures)
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return sub, err
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

func (w *Wallet) confirmUnitTx(ctx context.Context, sub *submittedTx, timeout uint64) error {
	submissions := make(map[string]*submittedTx, 1)
	submissions[sub.id.String()] = sub
	return w.confirmUnitsTx(ctx, submissions, timeout)
}

func (w *Wallet) confirmUnitsTx(ctx context.Context, subs map[string]*submittedTx, maxTimeout uint64) error {
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
