package tokens

import (
	"context"
	"crypto"
	"crypto/rand"
	"fmt"
	"reflect"
	"sort"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type txPreprocessor func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error

func (w *Wallet) newType(ctx context.Context, accNr uint64, attrs AttrWithSubTypeCreationInputs, typeId twb.TokenTypeID, subtypePredicateArgs []*PredicateInput) (twb.TokenTypeID, error) {
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	acc, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}
	sub, err := w.prepareTx(ctx, wallet.UnitID(typeId), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
	err = sub.ToBatch(w.backend, acc.PubKey).SendTx(ctx, w.confirmTx)
	if err != nil {
		return nil, err
	}
	return twb.TokenTypeID(sub.UnitID), nil
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

	sub, err := w.prepareTx(ctx, wallet.UnitID(tokenId), attrs, key, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
	err = sub.ToBatch(w.backend, key.PubKey).SendTx(ctx, w.confirmTx)
	if err != nil {
		return nil, err
	}
	return twb.TokenID(sub.UnitID), nil
}

func RandomID() (wallet.UnitID, error) {
	id := make([]byte, 32)
	_, err := rand.Read(id)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (w *Wallet) prepareTx(ctx context.Context, unitId wallet.UnitID, attrs proto.Message, ac *account.AccountKey, txps txPreprocessor) (*txsubmitter.TxSubmission, error) {
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
	tx.ServerMetadata = &txsystem.ServerMetadata{Fee: 0} // TODO server metadata changes the hash
	gtx, err = ttxs.NewGenericTx(tx)
	if err != nil {
		return nil, err
	}
	txSub := &txsubmitter.TxSubmission{
		UnitID:      unitId,
		Transaction: tx,
		TxHash:      gtx.Hash(crypto.SHA256),
	}
	return txSub, nil
}

func signTx(gtx txsystem.GenericTransaction, ac *account.AccountKey) (wallet.Predicate, error) {
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
		NewBearer:                    BearerPredicateFromPubKey(receiverPubKey),
		Value:                        token.Amount,
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

func newNonFungibleTransferTxAttrs(token *twb.TokenUnit, receiverPubKey []byte) *ttxs.TransferNonFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating NFT transfer with bl=%X", token.TxHash))
	return &ttxs.TransferNonFungibleTokenAttributes{
		NftType:                      token.TypeID,
		NewBearer:                    BearerPredicateFromPubKey(receiverPubKey),
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

func bearerPredicateFromHash(receiverPubKeyHash []byte) wallet.Predicate {
	if receiverPubKeyHash != nil {
		return script.PredicatePayToPublicKeyHashDefault(receiverPubKeyHash)
	}
	return script.PredicateAlwaysTrue()
}

func BearerPredicateFromPubKey(receiverPubKey wallet.PubKey) wallet.Predicate {
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
		NewBearer:                    BearerPredicateFromPubKey(receiverPubKey),
		TargetValue:                  amount,
		RemainingValue:               token.Amount - amount,
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
}

func newBurnTxAttrs(token *twb.TokenUnit, targetStateHash []byte) *ttxs.BurnFungibleTokenAttributes {
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
func (w *Wallet) doSendMultiple(ctx context.Context, amount uint64, tokens []*twb.TokenUnit, acc *account.AccountKey, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) error {
	var accumulatedSum uint64
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Amount > tokens[j].Amount
	})

	batch := txsubmitter.NewBatch(acc.PubKey, w.backend)

	for _, t := range tokens {
		remainingAmount := amount - accumulatedSum
		sub, err := w.prepareSplitOrTransferTx(ctx, acc, remainingAmount, t, receiverPubKey, invariantPredicateArgs)
		if err != nil {
			return err
		}
		batch.Add(sub)
		accumulatedSum += t.Amount
		if accumulatedSum >= amount {
			break
		}
	}
	return batch.SendTx(ctx, w.confirmTx)
}

func (w *Wallet) prepareSplitOrTransferTx(ctx context.Context, acc *account.AccountKey, amount uint64, token *twb.TokenUnit, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (*txsubmitter.TxSubmission, error) {
	var attrs AttrWithInvariantPredicateInputs
	if amount >= token.Amount {
		attrs = newFungibleTransferTxAttrs(token, receiverPubKey)
	} else {
		attrs = newSplitTxAttrs(token, amount, receiverPubKey)
	}
	sub, err := w.prepareTx(ctx, wallet.UnitID(token.ID), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: timeout},
		// OwnerProof is added after whole transaction is built
	}
}
