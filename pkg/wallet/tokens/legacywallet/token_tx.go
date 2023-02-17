package legacywallet

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"math/rand"
	"reflect"
	"sort"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	submittedTx struct {
		id        TokenID
		tx        *txsystem.Transaction
		confirmed bool
	}

	submissionSet struct {
		submissions map[string]*submittedTx
		maxTimeout  uint64
	}

	txPreprocessor func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error
)

func newSubmissionSet() *submissionSet {
	return &submissionSet{
		submissions: make(map[string]*submittedTx, 1),
		maxTimeout:  0,
	}
}

func (s *submittedTx) confirm() {
	s.confirmed = true
}

func (s *submissionSet) confirmed() bool {
	for _, sub := range s.submissions {
		if !sub.confirmed {
			return false
		}
	}
	return true
}

func (s *submissionSet) add(sub *submittedTx) *submissionSet {
	if sub.tx.Timeout() > s.maxTimeout {
		s.maxTimeout = sub.tx.Timeout()
	}
	s.submissions[sub.id.String()] = sub
	return s
}

func (w *Wallet) readTx(txc TokenTxContext, tx *txsystem.Transaction, b *block.Block, accNr uint64, key *account.KeyHashes) error {
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
		err = w.addTokenTypeWithProof(&TokenUnitType{
			ID:            id,
			Kind:          FungibleTokenType,
			Symbol:        ctx.Symbol(),
			ParentTypeID:  ctx.ParentTypeID(),
			DecimalPlaces: ctx.DecimalPlaces(),
		}, b, tx, txc)
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
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				Kind:     FungibleToken,
				TypeID:   ctx.TypeID(),
				Amount:   ctx.Value(),
				Backlink: txHash,
				Symbol:   tType.Symbol,
			}, b, tx, txc)
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
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				TypeID:   ctx.TypeID(),
				Kind:     FungibleToken,
				Amount:   ctx.Value(),
				Symbol:   tokenInfo.Symbol,
				Backlink: txHash,
			}, b, tx, txc)
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
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				Symbol:   tok.Symbol,
				TypeID:   tok.TypeID,
				Kind:     tok.Kind,
				Amount:   tok.Amount - ctx.TargetValue(),
				Backlink: txHash,
			}, b, tx, txc)
			if err != nil {
				return err
			}
		} else {
			ti, err := txc.GetTokenType(ctx.TypeID())
			if err != nil {
				return err
			}
			if ti == nil {
				return errors.Errorf("split tx: token type with id=%X not found, token id=%X", ctx.TypeID(), id)
			}
			tokenInfo = ti
		}

		if checkOwner(accNr, key, ctx.NewBearer()) {
			newId := txutil.SameShardIDBytes(ctx.UnitID(), ctx.HashForIDCalculation(crypto.SHA256))
			log.Info(fmt.Sprintf("SplitFungibleToken: adding new unit from split, new UnitId=%X", newId))
			err := w.addTokenWithProof(accNr, &TokenUnit{
				ID:       newId,
				Symbol:   tokenInfo.GetSymbol(),
				TypeID:   tokenInfo.GetTypeId(),
				Kind:     FungibleToken,
				Amount:   ctx.TargetValue(),
				Backlink: txHash,
			}, b, tx, txc)
			if err != nil {
				return err
			}
		}
	case tokens.BurnFungibleToken:
		log.Info("Token tx: BurnFungibleToken")
		tok, err := txc.GetToken(accNr, id)
		if err != nil {
			return err
		}
		if tok != nil {
			log.Info("Token was burned on account #", accNr)
			if tok.Amount != ctx.Value() {
				return fmt.Errorf("expected burned amount: %v, got %v. token id='%X', type id='%X'", tok.Amount, ctx.Value(), tok.ID, tok.TypeID)
			}
			tok.Burned = true
			err = w.addTokenWithProof(accNr, tok, b, tx, txc)
			if err != nil {
				return err
			}
		}
	case tokens.JoinFungibleToken:
		log.Info("Token tx: JoinFungibleToken")
		joinedToken, err := txc.GetToken(accNr, id)
		if err != nil {
			return err
		}
		if joinedToken == nil {
			return nil
		}
		bl := joinedToken.Backlink
		// burned tokens must be on the same account as the target unit
		var burnedValue uint64
		for _, burnTx := range ctx.BurnTransactions() {
			burnedID := util.Uint256ToBytes(burnTx.UnitID())
			burnedToken, err := txc.GetToken(accNr, burnedID)
			if err != nil {
				return err
			}
			if burnedToken == nil {
				return fmt.Errorf("burned token with id '%X' not found", burnedID)
			}
			if !burnedToken.Burned {
				return fmt.Errorf("token with id '%X' is expected to be burned, but it is not", burnedID)
			}
			if !bytes.Equal(bl, burnTx.Nonce()) {
				return fmt.Errorf("expected burned token's nonce '%X', got %X", bl, burnTx.Nonce())
			}
			err = txc.RemoveToken(accNr, burnedID)
			if err != nil {
				return err
			}
			burnedValue += burnTx.Value()
		}
		joinedToken.Amount += burnedValue
		joinedToken.Backlink = txHash
		err = w.addTokenWithProof(accNr, joinedToken, b, tx, txc)
		if err != nil {
			return err
		}
	case tokens.CreateNonFungibleTokenType:
		log.Info("Token tx: CreateNonFungibleTokenType")
		err := w.addTokenTypeWithProof(&TokenUnitType{
			ID:           id,
			Kind:         NonFungibleTokenType,
			Symbol:       ctx.Symbol(),
			ParentTypeID: ctx.ParentTypeID(),
		}, b, tx, txc)
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
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				Kind:     NonFungibleToken,
				TypeID:   ctx.NFTTypeID(),
				URI:      ctx.URI(),
				Backlink: txHash,
				Symbol:   tType.Symbol,
			}, b, tx, txc)
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
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				TypeID:   ctx.NFTTypeID(),
				Kind:     NonFungibleToken,
				Backlink: txHash,
				Symbol:   tType.Symbol,
			}, b, tx, txc)
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
		tok, err := txc.GetToken(accNr, id)
		if err != nil {
			return err
		}
		if tok != nil {
			tok.Backlink = txHash
			if err = w.addTokenWithProof(accNr, tok, b, tx, txc); err != nil {
				return err
			}
		}
	default:
		log.Warning(fmt.Sprintf("received unknown token transaction type, skipped processing: %s", ctx))
		return nil
	}
	return nil
}

func checkOwner(accNr uint64, pubkeyHashes *account.KeyHashes, bearerPredicate []byte) bool {
	if accNr == alwaysTrueTokensAccountNumber {
		return bytes.Equal(script.PredicateAlwaysTrue(), bearerPredicate)
	} else {
		return account.VerifyP2PKHOwner(pubkeyHashes, bearerPredicate)
	}
}

func (w *Wallet) newType(ctx context.Context, attrs AttrWithSubTypeCreationInputs, typeId TokenTypeID, subtypePredicateArgs []*PredicateInput) (TokenID, error) {
	sub, err := w.sendTx(TokenID(typeId), attrs, nil, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
	return sub.id, w.syncToUnit(ctx, sub)
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

func (w *Wallet) newToken(ctx context.Context, accNr uint64, attrs MintAttr, tokenId TokenID, mintPredicateArgs []*PredicateInput) (TokenID, error) {
	var keyHash []byte
	if accNr > 0 {
		accIdx := accNr - 1
		key, err := w.am.GetAccountKey(accIdx)
		if err != nil {
			return nil, err
		}
		keyHash = key.PubKeyHash.Sha256
	}
	attrs.SetBearer(bearerPredicateFromHash(keyHash))

	sub, err := w.sendTx(tokenId, attrs, nil, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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

	return sub.id, w.syncToUnit(ctx, sub)
}

func RandomID() (TokenID, error) {
	id := make([]byte, 32)
	_, err := rand.Read(id)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (w *Wallet) sendTx(unitId TokenID, attrs proto.Message, ac *account.AccountKey, txps txPreprocessor) (*submittedTx, error) {
	txSub := &submittedTx{id: unitId}
	if unitId == nil {
		id, err := RandomID()
		if err != nil {
			return txSub, err
		}
		txSub.id = id
	}
	log.Info(fmt.Sprintf("Sending token tx, UnitID=%X, attributes: %v", txSub.id, reflect.TypeOf(attrs)))

	_, roundNumber, err := w.sdk.GetMaxBlockNumber()
	if err != nil {
		return txSub, err
	}
	tx := createTx(w.SystemID(), txSub.id, roundNumber+txTimeoutBlockCount)
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
	err = w.sdk.SendTransaction(nil, tx, nil)
	if err != nil {
		return txSub, err
	}
	txSub.tx = tx
	return txSub, nil
}

func signTx(gtx txsystem.GenericTransaction, ac *account.AccountKey) (tokens.Predicate, error) {
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
		InvariantPredicateSignatures: nil,
	}
}

func newNonFungibleTransferTxAttrs(token *TokenUnit, receiverPubKey []byte) *tokens.TransferNonFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating NFT transfer with bl=%X", token.Backlink))
	return &tokens.TransferNonFungibleTokenAttributes{
		NftType:                      token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		Backlink:                     token.Backlink,
		InvariantPredicateSignatures: nil,
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

func newSplitTxAttrs(token *TokenUnit, amount uint64, receiverPubKey []byte) *tokens.SplitFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating split with bl=%X, new value=%v", token.Backlink, amount))
	return &tokens.SplitFungibleTokenAttributes{
		Type:                         token.TypeID,
		NewBearer:                    bearerPredicateFromPubKey(receiverPubKey),
		TargetValue:                  amount,
		RemainingValue:               token.Amount - amount,
		Backlink:                     token.Backlink,
		InvariantPredicateSignatures: nil,
	}
}

func newBurnTxAttrs(tok *TokenUnit, targetStateHash []byte) *tokens.BurnFungibleTokenAttributes {
	log.Info(fmt.Sprintf("Creating burn tx of unit=%X with bl=%X, new value=%v", tok.ID, tok.Backlink, tok.Amount))
	return &tokens.BurnFungibleTokenAttributes{
		Type:                         tok.TypeID,
		Value:                        tok.Amount,
		Nonce:                        targetStateHash,
		Backlink:                     tok.Backlink,
		InvariantPredicateSignatures: nil,
	}
}

// assumes there's sufficient balance for the given amount, sends transactions immediately
func (w *Wallet) doSendMultiple(amount uint64, tokens []*TokenUnit, acc *account.AccountKey, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (*submissionSet, error) {
	var accumulatedSum uint64
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Amount > tokens[j].Amount
	})
	submissions := newSubmissionSet()
	for _, t := range tokens {
		remainingAmount := amount - accumulatedSum
		sub, err := w.sendSplitOrTransferTx(acc, remainingAmount, t, receiverPubKey, invariantPredicateArgs)
		if err != nil {
			return submissions, err
		}
		submissions.add(sub)
		accumulatedSum += t.Amount
		if accumulatedSum >= amount {
			break
		}
	}
	return submissions, nil
}

func (w *Wallet) sendSplitOrTransferTx(acc *account.AccountKey, amount uint64, token *TokenUnit, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (*submittedTx, error) {
	var attrs AttrWithInvariantPredicateInputs
	if amount >= token.Amount {
		attrs = newFungibleTransferTxAttrs(token, receiverPubKey)
	} else {
		attrs = newSplitTxAttrs(token, amount, receiverPubKey)
	}
	sub, err := w.sendTx(token.ID, attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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

func createTx(systemId, unitId []byte, timeout uint64) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              systemId,
		UnitId:                unitId,
		TransactionAttributes: new(anypb.Any),
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: timeout},
		// OwnerProof is added after whole transaction is built
	}
}

func (w *Wallet) addTokenTypeWithProof(unit *TokenUnitType, b *block.Block, tx *txsystem.Transaction, txc TokenTxContext) error {
	proof, err := w.createProof(unit.ID, b, tx)
	if err != nil {
		return err
	}
	unit.Proof = proof
	return txc.AddTokenType(unit)
}

func (w *Wallet) addTokenWithProof(accountNumber uint64, unit *TokenUnit, b *block.Block, tx *txsystem.Transaction, txc TokenTxContext) error {
	proof, err := w.createProof(unit.ID, b, tx)
	if err != nil {
		return err
	}
	unit.Proof = proof
	return txc.SetToken(accountNumber, unit)
}

func (w *Wallet) createProof(unitID []byte, b *block.Block, tx *txsystem.Transaction) (*Proof, error) {
	if b == nil {
		return nil, nil
	}
	gblock, err := b.ToGenericBlock(w.txs)
	if err != nil {
		return nil, err
	}
	proof, err := block.NewPrimaryProof(gblock, unitID, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	return &Proof{
		BlockNumber: b.UnicityCertificate.InputRecord.RoundNumber,
		Tx:          tx,
		Proof:       proof,
	}, nil
}
