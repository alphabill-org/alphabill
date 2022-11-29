package tokens

import (
	"bytes"
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
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	submittedTx struct {
		id      TokenID
		timeout uint64
	}

	txPreprocessor func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error
)

func (w *Wallet) readTx(txc TokenTxContext, tx *txsystem.Transaction, accNr uint64, key *wallet.KeyHashes) error {
	gtx, err := w.txs.ConvertTx(tx)
	if err != nil {
		return err
	}
	id := util.Uint256ToBytes(gtx.UnitID())
	txHash := gtx.Hash(crypto.SHA256)
	log.Info(fmt.Sprintf("Converted tx: UnitId=%X, TxId=%X", id, txHash))

	switch ctx := gtx.(type) {
	case tokens.CreateFungibleTokenType:
		log.Info("CreateFungibleTokenType tx")
		err := txc.AddTokenType(&TokenUnitType{
			ID:            id,
			Kind:          FungibleTokenType,
			Symbol:        ctx.Symbol(),
			ParentTypeID:  ctx.ParentTypeID(),
			DecimalPlaces: ctx.DecimalPlaces(),
		})
		if err != nil {
			return err
		}
	case tokens.MintFungibleToken:
		log.Info("MintFungibleToken tx")
		if checkOwner(accNr, key, ctx.Bearer()) {
			tType, err := txc.GetTokenType(ctx.TypeID())
			if err != nil {
				return err
			}
			if tType == nil {
				return errors.Errorf("mint fungible token tx: token type with id=%X not found, token id=%X", ctx.TypeID(), id)
			}
			err = txc.SetToken(accNr, &TokenUnit{
				ID:       id,
				Kind:     FungibleToken,
				TypeID:   ctx.TypeID(),
				Amount:   ctx.Value(),
				Backlink: make([]byte, crypto.SHA256.Size()), //zerohash
				Symbol:   tType.Symbol,
			})
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.TransferFungibleToken:
		log.Info("TransferFungibleToken tx")
		if checkOwner(accNr, key, ctx.NewBearer()) {
			tokenInfo, err := txc.GetTokenType(ctx.TypeID())
			if err != nil {
				return err
			}
			if tokenInfo == nil {
				return errors.Errorf("fungible transfer tx: token type with id=%X not found, token id=%X", ctx.TypeID(), id)
			}
			err = txc.SetToken(accNr, &TokenUnit{
				ID:       id,
				TypeID:   ctx.TypeID(),
				Kind:     FungibleToken,
				Amount:   ctx.Value(),
				Symbol:   tokenInfo.Symbol,
				Backlink: txHash,
			})
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.SplitFungibleToken:
		log.Info("SplitFungibleToken tx")
		tok, err := txc.GetToken(accNr, id)
		if err != nil {
			return err
		}
		var tokenInfo TokenTypeInfo
		if tok != nil {
			tokenInfo = tok
			log.Info("SplitFungibleToken updating existing unit")
			if !bytes.Equal(tok.TypeID, ctx.TypeID()) {
				return errors.Errorf("split tx: type id does not match (received '%X', expected '%X'), token id=%X", ctx.TypeID(), tok.TypeID, tok.ID)
			}
			remainingValue := tok.Amount - ctx.TargetValue()
			if ctx.RemainingValue() != remainingValue {
				return errors.Errorf("split tx: invalid remaining amount (received '%v', expected '%v'), token id=%X", ctx.RemainingValue(), remainingValue, tok.ID)
			}
			if err = txc.SetToken(accNr, &TokenUnit{
				ID:       id,
				Symbol:   tok.Symbol,
				TypeID:   tok.TypeID,
				Kind:     tok.Kind,
				Amount:   tok.Amount - ctx.TargetValue(),
				Backlink: txHash,
			}); err != nil {
				return err
			}
		} else {
			tokenInfo, err = txc.GetTokenType(ctx.TypeID())
			if err != nil {
				return err
			}
			if tokenInfo == nil {
				return errors.Errorf("split tx: token type with id=%X not found, token id=%X", ctx.TypeID(), id)
			}
		}

		if checkOwner(accNr, key, ctx.NewBearer()) {
			newId := txutil.SameShardIDBytes(ctx.UnitID(), ctx.HashForIDCalculation(crypto.SHA256))
			log.Info(fmt.Sprintf("SplitFungibleToken: adding new unit from split, new UnitId=%X", newId))
			err := txc.SetToken(accNr, &TokenUnit{
				ID:       newId,
				Symbol:   tokenInfo.GetSymbol(),
				TypeID:   tokenInfo.GetTypeId(),
				Kind:     FungibleToken,
				Amount:   ctx.TargetValue(),
				Backlink: make([]byte, crypto.SHA256.Size()), //zerohash
			})
			if err != nil {
				return err
			}
		}
	case tokens.BurnFungibleToken:
		log.Info("Token tx: BurnFungibleToken")
		panic("not implemented") // TODO
	case tokens.JoinFungibleToken:
		log.Info("Token tx: JoinFungibleToken")
		panic("not implemented") // TODO
	case tokens.CreateNonFungibleTokenType:
		log.Info("Token tx: CreateNonFungibleTokenType")
		err := txc.AddTokenType(&TokenUnitType{
			ID:           id,
			Kind:         NonFungibleTokenType,
			Symbol:       ctx.Symbol(),
			ParentTypeID: ctx.ParentTypeID(),
		})
		if err != nil {
			return err
		}
	case tokens.MintNonFungibleToken:
		log.Info("Token tx: MintNonFungibleToken")
		if checkOwner(accNr, key, ctx.Bearer()) {
			tType, err := txc.GetTokenType(ctx.NFTTypeID())
			if err != nil {
				return err
			}
			if tType == nil {
				return errors.Errorf("mint nft tx: token type with id=%X not found, token id=%X", ctx.NFTTypeID(), id)
			}
			err = txc.SetToken(accNr, &TokenUnit{
				ID:       id,
				Kind:     NonFungibleToken,
				TypeID:   ctx.NFTTypeID(),
				URI:      ctx.URI(),
				Backlink: make([]byte, crypto.SHA256.Size()), //zerohash
				Symbol:   tType.Symbol,
			})
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.TransferNonFungibleToken:
		log.Info("Token tx: TransferNonFungibleToken")
		if checkOwner(accNr, key, ctx.NewBearer()) {
			tType, err := txc.GetTokenType(ctx.NFTTypeID())
			if err != nil {
				return err
			}
			if tType == nil {
				return errors.Errorf("transfer nft tx: token type with id=%X not found, token id=%X", ctx.NFTTypeID(), id)
			}
			err = txc.SetToken(accNr, &TokenUnit{
				ID:       id,
				TypeID:   ctx.NFTTypeID(),
				Kind:     NonFungibleToken,
				Backlink: txHash,
				Symbol:   tType.Symbol,
			})
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.UpdateNonFungibleToken:
		log.Info("Token tx: UpdateNonFungibleToken")
		panic("not implemented") // TODO
	default:
		log.Warning(fmt.Sprintf("received unknown token transaction type, skipped processing: %s", ctx))
		return nil
	}
	return nil
}

func checkOwner(accNr uint64, pubkeyHashes *wallet.KeyHashes, bearerPredicate []byte) bool {
	if accNr == alwaysTrueTokensAccountNumber {
		return bytes.Equal(script.PredicateAlwaysTrue(), bearerPredicate)
	} else {
		return wallet.VerifyP2PKHOwner(pubkeyHashes, bearerPredicate)
	}
}

func (w *Wallet) newType(ctx context.Context, attrs tokens.AttrWithSubTypeCreationInputs, typeId TokenTypeID, subtypePredicateArgs []*PredicateInput) (TokenID, error) {
	sub, err := w.sendTx(TokenID(typeId), attrs, nil, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.GetAccountManager(), subtypePredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.SetSubTypeCreationPredicateSignatures(signatures)
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return nil, err
	}
	return sub.id, w.syncToUnit(ctx, sub.id, sub.timeout)
}

func preparePredicateSignatures(am wallet.AccountManager, args []*PredicateInput, gtx txsystem.GenericTransaction) ([][]byte, error) {
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

func (w *Wallet) newToken(ctx context.Context, accNr uint64, attrs tokens.MintAttr, tokenId TokenID, mintPredicateArgs []*PredicateInput) (TokenID, error) {
	var keyHash []byte
	if accNr > 0 {
		accIdx := accNr - 1
		key, err := w.mw.GetAccountKey(accIdx)
		if err != nil {
			return nil, err
		}
		keyHash = key.PubKeyHash.Sha256
	}
	attrs.SetBearer(bearerPredicateFromHash(keyHash))

	sub, err := w.sendTx(tokenId, attrs, nil, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.GetAccountManager(), mintPredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.SetTokenCreationPredicateSignatures(signatures)
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return nil, err
	}

	return sub.id, w.syncToUnit(ctx, sub.id, sub.timeout)
}

func RandomID() (TokenID, error) {
	id := make([]byte, 32)
	_, err := rand.Read(id)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (w *Wallet) sendTx(unitId TokenID, attrs proto.Message, ac *wallet.AccountKey, txps txPreprocessor) (*submittedTx, error) {
	txSub := &submittedTx{id: unitId}
	if unitId == nil {
		id, err := RandomID()
		if err != nil {
			return txSub, err
		}
		txSub.id = id
	}
	log.Info(fmt.Sprintf("Sending token tx, UnitID=%X, attributes: %v", txSub.id, reflect.TypeOf(attrs)))

	blockNumber, err := w.mw.GetMaxBlockNumber()
	if err != nil {
		return txSub, err
	}
	tx := createTx(txSub.id, blockNumber+txTimeoutBlockCount)
	err = anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	if err != nil {
		return txSub, err
	}
	gtx, err := tokens.NewGenericTx(tx)
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
	err = w.mw.SendTransaction(nil, tx, nil)
	if err != nil {
		return txSub, err
	}
	txSub.timeout = tx.Timeout
	return txSub, nil
}

func signTx(gtx txsystem.GenericTransaction, ac *wallet.AccountKey) (tokens.Predicate, error) {
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

func newFungibleTransferTxAttrs(token *TokenUnit, receiverPubKey []byte) *tokens.TransferFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating transfer with bl=%X", token.Backlink))
	return &tokens.TransferFungibleTokenAttributes{
		Type:                         token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		Value:                        token.Amount,
		Backlink:                     token.Backlink,
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
}

func newNonFungibleTransferTxAttrs(token *TokenUnit, receiverPubKey []byte) *tokens.TransferNonFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating NFT transfer with bl=%X", token.Backlink))
	return &tokens.TransferNonFungibleTokenAttributes{
		NftType:                      token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		Backlink:                     token.Backlink,
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
}

func bearerPredicateFromHash(receiverPubKeyHash []byte) tokens.Predicate {
	if receiverPubKeyHash != nil {
		return script.PredicatePayToPublicKeyHashDefault(receiverPubKeyHash)
	}
	return script.PredicateAlwaysTrue()
}

func bearerPredicateFromPubKey(receiverPubKey PublicKey) tokens.Predicate {
	if receiverPubKey == nil {
		return bearerPredicateFromHash(nil)
	}
	return bearerPredicateFromHash(hash.Sum256(receiverPubKey))
}

func (w *Wallet) transfer(ctx context.Context, ac *wallet.AccountKey, token *TokenUnit, receiverPubKey []byte) error {
	sub, err := w.sendTx(token.ID, newFungibleTransferTxAttrs(token, receiverPubKey), ac, nil)
	if err != nil {
		return err
	}

	return w.syncToUnit(ctx, token.ID, sub.timeout)
}

func newSplitTxAttrs(token *TokenUnit, amount uint64, receiverPubKey []byte) *tokens.SplitFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating split with bl=%X, new value=%v", token.Backlink, amount))
	return &tokens.SplitFungibleTokenAttributes{
		Type:                         token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		TargetValue:                  amount,
		RemainingValue:               token.Amount - amount,
		Backlink:                     token.Backlink,
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
}

// assumes there's sufficient balance for the given amount, sends transactions immediately
func (w *Wallet) doSendMultiple(amount uint64, tokens []*TokenUnit, acc *wallet.AccountKey, receiverPubKey []byte) (map[string]*submittedTx, uint64, error) {
	var accumulatedSum uint64
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Amount > tokens[j].Amount
	})
	var maxTimeout uint64 = 0
	submissions := make(map[string]*submittedTx, 2)
	for _, t := range tokens {
		remainingAmount := amount - accumulatedSum
		sub, err := w.sendSplitOrTransferTx(acc, remainingAmount, t, receiverPubKey)
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

func (w *Wallet) sendSplitOrTransferTx(acc *wallet.AccountKey, amount uint64, token *TokenUnit, receiverPubKey []byte) (*submittedTx, error) {
	var attrs proto.Message
	if amount >= token.Amount {
		attrs = newFungibleTransferTxAttrs(token, receiverPubKey)
	} else {
		attrs = newSplitTxAttrs(token, amount, receiverPubKey)
	}
	sub, err := w.sendTx(token.ID, attrs, acc, nil)
	if err != nil {
		return sub, err
	}
	return sub, nil
}

func createTx(unitId []byte, timeout uint64) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              tokens.DefaultTokenTxSystemIdentifier,
		UnitId:                unitId,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		// OwnerProof is added after whole transaction is built
	}
}
