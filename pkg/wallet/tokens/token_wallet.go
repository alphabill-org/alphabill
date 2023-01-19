package tokens

import (
	"context"
	goerrors "errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	uriMaxSize  = 4 * 1024
	dataMaxSize = 64 * 1024
)

var (
	ErrInvalidBlockSystemID = goerrors.New("invalid system identifier")
	ErrAttributesMissing    = goerrors.New("attributes missing")
	ErrInvalidURILength     = fmt.Errorf("URI exceeds the maximum allowed size of %v bytes", uriMaxSize)
	ErrInvalidDataLength    = fmt.Errorf("data exceeds the maximum allowed size of %v bytes", dataMaxSize)
)

type (
	Wallet struct {
		mw            *money.Wallet
		db            *tokensDb
		txs           block.TxConverter
		sync          bool
		blockListener wallet.BlockProcessor
	}
)

func Load(mw *money.Wallet, sync bool) (*Wallet, error) {
	config := mw.GetConfig()
	walletDir, err := config.GetWalletDir()
	if err != nil {
		return nil, err
	}

	db, err := openTokensDb(walletDir)
	if err != nil {
		return nil, err
	}
	txs, err := tokens.New()
	if err != nil {
		return nil, err
	}
	w := &Wallet{mw: mw, db: db, txs: txs, sync: sync, blockListener: nil}
	w.mw.Wallet = wallet.New().
		SetBlockProcessor(w).
		SetABClient(mw.AlphabillClient).
		Build()
	return w, nil
}

func (w *Wallet) SystemID() []byte {
	// TODO: return the default "AlphaBill Token System ID" for now
	// but in the future this should come from config (w.mw.SystemID()?)
	return tokens.DefaultTokenTxSystemIdentifier
}

func (w *Wallet) GetAccountManager() wallet.AccountManager {
	return w.mw
}

func (w *Wallet) Shutdown() {
	w.mw.Shutdown()
	if w.db != nil {
		w.db.Close()
	}
}

func (w *Wallet) NewFungibleType(ctx context.Context, attrs *tokens.CreateFungibleTokenTypeAttributes, typeId TokenTypeID, subtypePredicateArgs []*PredicateInput) (TokenID, error) {
	log.Info("Creating new fungible token type")
	parentType, err := w.db.Do().GetTokenType(attrs.ParentTypeId)
	if err != nil {
		return nil, err
	}
	if parentType != nil && parentType.DecimalPlaces != attrs.DecimalPlaces {
		return nil, errors.Errorf("invalid decimal places. allowed %v, got %v", parentType.DecimalPlaces, attrs.DecimalPlaces)
	}
	return w.newType(ctx, attrs, typeId, subtypePredicateArgs)
}

func (w *Wallet) NewNonFungibleType(ctx context.Context, attrs *tokens.CreateNonFungibleTokenTypeAttributes, typeId TokenTypeID, subtypePredicateArgs []*PredicateInput) (TokenID, error) {
	log.Info("Creating new NFT type")
	return w.newType(ctx, attrs, typeId, subtypePredicateArgs)
}

func (w *Wallet) NewFungibleToken(ctx context.Context, accNr uint64, attrs *tokens.MintFungibleTokenAttributes, mintPredicateArgs []*PredicateInput) (TokenID, error) {
	log.Info("Creating new fungible token")
	return w.newToken(ctx, accNr, attrs, nil, mintPredicateArgs)
}

func (w *Wallet) NewNFT(ctx context.Context, accNr uint64, attrs *tokens.MintNonFungibleTokenAttributes, tokenId TokenID, mintPredicateArgs []*PredicateInput) (TokenID, error) {
	log.Info("Creating new NFT")
	if attrs == nil {
		return nil, ErrAttributesMissing
	}
	if len(attrs.Uri) > uriMaxSize {
		return nil, ErrInvalidURILength
	}
	if attrs.Uri != "" && !util.IsValidURI(attrs.Uri) {
		return nil, fmt.Errorf("URI '%s' is invalid", attrs.Uri)
	}
	if len(attrs.Data) > dataMaxSize {
		return nil, ErrInvalidDataLength
	}
	return w.newToken(ctx, accNr, attrs, tokenId, mintPredicateArgs)
}

func (w *Wallet) ListTokenTypes(ctx context.Context, kind TokenKind) ([]*TokenUnitType, error) {
	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}
	tokenTypes, err := w.db.Do().GetTokenTypes()
	if err != nil {
		return nil, err
	}
	if kind&Any > 0 {
		return tokenTypes, nil
	}
	res := make([]*TokenUnitType, 0)
	// filter out specific type requested
	for _, tt := range tokenTypes {
		if tt.Kind&kind == kind {
			res = append(res, tt)
		}
	}
	return res, nil
}

// GetTokenType returns non-nil TokenUnitType or error if not found or other issues
func (w *Wallet) GetTokenType(ctx context.Context, typeId TokenTypeID) (*TokenUnitType, error) {
	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}
	tt, err := w.db.Do().GetTokenType(typeId)
	if err != nil {
		return nil, err
	}
	if tt == nil {
		return nil, fmt.Errorf("error token type %X not found", typeId)
	}
	return tt, nil
}

// ListTokens specify accountNumber=-1 to list tokens from all accounts
func (w *Wallet) ListTokens(ctx context.Context, kind TokenKind, accountNumber int) (map[uint64][]*TokenUnit, error) {

	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}

	var pubKeys [][]byte
	singleKey := false
	if accountNumber > AllAccounts+1 {
		key, err := w.mw.GetPublicKey(uint64(accountNumber - 1))
		if err != nil {
			return nil, err
		}
		pubKeys = append(pubKeys, key)
		singleKey = true
	} else if accountNumber != alwaysTrueTokensAccountNumber {
		pubKeys, err = w.mw.GetPublicKeys()
		if err != nil {
			return nil, err
		}
	}

	// account number -> list of its tokens
	res := make(map[uint64][]*TokenUnit, 0)

	fetchTokens := func(accNr uint64) error {
		tokenz, err := w.db.Do().GetTokens(accNr)
		if err != nil {
			return err
		}
		for _, tok := range tokenz {
			if kind&Any > 0 || tok.Kind&kind == kind {
				units, found := res[accNr]
				if found {
					res[accNr] = append(units, tok)
				} else {
					res[accNr] = []*TokenUnit{tok}
				}
			}
		}
		return nil
	}

	if singleKey {
		return res, fetchTokens(uint64(accountNumber))
	}
	// NB! n=0 is a special index for always true predicates, thus iteration goes until len, not len-1
	for n := 0; n <= len(pubKeys); n++ {
		err := fetchTokens(uint64(n))
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (w *Wallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenId TokenID, receiverPubKey PublicKey, invariantPredicateArgs []*PredicateInput) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	t, err := w.db.Do().GetToken(accountNumber, tokenId)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("token with id=%X not found under account #%v", tokenId, accountNumber)
	}

	attrs := newNonFungibleTransferTxAttrs(t, receiverPubKey)
	sub, err := w.sendTx(tokenId, attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.InvariantPredicateSignatures = signatures
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return err
	}

	return w.syncToUnit(ctx, sub)
}

func (w *Wallet) SendFungible(ctx context.Context, accountNumber uint64, typeId TokenTypeID, targetAmount uint64, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	tokens, err := w.db.Do().GetTokens(accountNumber)
	if err != nil {
		return err
	}
	fungibleTokens := make([]*TokenUnit, 0)
	var totalBalance uint64 = 0
	// find the best unit candidate for transfer or split, value must be equal or larger than the target amount
	var closestMatch *TokenUnit = nil
	for _, token := range tokens {
		if token.IsFungible() && typeId.equal(token.TypeID) {
			fungibleTokens = append(fungibleTokens, token)
			totalBalance += token.Amount
			if closestMatch == nil {
				closestMatch = token
			} else {
				prevDiff := closestMatch.Amount - targetAmount
				currDiff := token.Amount - targetAmount
				// this should work with overflow nicely
				if prevDiff > currDiff {
					closestMatch = token
				}
			}
		}
	}
	if targetAmount > totalBalance {
		return fmt.Errorf("insufficient value: got %v, need %v", totalBalance, targetAmount)
	}
	var submissions *submissionSet
	// optimization: first try to make a single operation instead of iterating through all tokens in doSendMultiple
	if closestMatch.Amount >= targetAmount {
		var sub *submittedTx
		sub, err = w.sendSplitOrTransferTx(acc, targetAmount, closestMatch, receiverPubKey, invariantPredicateArgs)
		submissions = newSubmissionSet().add(sub)
	} else {
		submissions, err = w.doSendMultiple(targetAmount, fungibleTokens, acc, receiverPubKey, invariantPredicateArgs)
	}

	// error might have happened, but some submissions could have succeeded
	syncErr := w.syncToUnits(ctx, submissions)

	if err != nil {
		return err
	}
	return syncErr
}

func (w *Wallet) getAccountKey(accountNumber uint64) (*wallet.AccountKey, error) {
	if accountNumber > 0 {
		return w.mw.GetAccountKey(accountNumber - 1)
	}
	return nil, nil
}

func (w *Wallet) UpdateNFTData(ctx context.Context, accountNumber uint64, tokenId []byte, data []byte, updatePredicateArgs []*PredicateInput) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	t, err := w.db.Do().GetToken(accountNumber, tokenId)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("token with id=%X not found under account #%v", tokenId, accountNumber)
	}

	attrs := &tokens.UpdateNonFungibleTokenAttributes{
		Data:                 data,
		Backlink:             t.Backlink,
		DataUpdateSignatures: nil,
	}

	sub, err := w.sendTx(tokenId, attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.GetAccountManager(), updatePredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.DataUpdateSignatures = signatures
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return err
	}

	return w.syncToUnit(ctx, sub)
}

func (w *Wallet) CollectDust(ctx context.Context, accountNumber int, tokenTypes []TokenTypeID, invariantPredicateArgs []*PredicateInput) error {
	if accountNumber == alwaysTrueTokensAccountNumber {
		return errors.New("invalid account number for dust collection (#0)")
	}

	var keys []*wallet.AccountKey
	var err error
	singleKey := false
	if accountNumber > AllAccounts+1 {
		key, err := w.mw.GetAccountKey(uint64(accountNumber - 1))
		if err != nil {
			return err
		}
		keys = append(keys, key)
		singleKey = true
	} else {
		keys, err = w.mw.GetAccountKeys()
		if err != nil {
			return err
		}
	}
	if singleKey {
		return w.collectDust(ctx, uint64(accountNumber), tokenTypes, invariantPredicateArgs)
	}
	for idx := range keys {
		err := w.collectDust(ctx, uint64(idx+1), tokenTypes, invariantPredicateArgs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) collectDust(ctx context.Context, accountNumber uint64, tokenTypes []TokenTypeID, invariantPredicateArgs []*PredicateInput) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	// find tokens to join
	allTokens, err := w.db.Do().GetTokens(accountNumber)
	if err != nil {
		return err
	}
	// group tokens by type
	var tokensByTypes = make(map[string][]*TokenUnit, len(tokenTypes))
	for _, tokenType := range tokenTypes {
		tokensByTypes[tokenType.String()] = make([]*TokenUnit, 0)
	}
	for _, tok := range allTokens {
		if !tok.IsFungible() {
			continue
		}
		tokenTypeStr := tok.TypeID.String()
		tokenz, found := tokensByTypes[tokenTypeStr]
		if !found {
			if len(tokenTypes) == 0 {
				// any type
				tokenz = make([]*TokenUnit, 0, 1)
			} else {
				continue
			}
		}
		tokensByTypes[tokenTypeStr] = append(tokenz, tok)
	}

	for k, v := range tokensByTypes {
		if len(v) < 2 { // not interested if tokens count is less than two
			delete(tokensByTypes, k)
			continue
		}
		// first token to be joined into
		targetToken := v[0]
		submissions := newSubmissionSet()
		// burn the rest
		for i := 1; i < len(v); i++ {
			token := v[i]
			attrs := newBurnTxAttrs(token, targetToken.Backlink)
			sub, err := w.sendTx(token.ID, attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
				signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, gtx)
				if err != nil {
					return err
				}
				attrs.SetInvariantPredicateSignatures(signatures)
				return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
			})
			if err != nil {
				return err
			}
			submissions.add(sub)
		}
		err = w.syncToUnits(ctx, submissions)
		if err != nil {
			return err
		}
		burnTxs := make([]*txsystem.Transaction, 0, 1)
		proofs := make([]*block.BlockProof, 0, 1)
		for _, tx := range submissions.submissions {
			tok, err := w.db.Do().GetToken(accountNumber, tx.id)
			if err != nil {
				return err
			}
			if !tok.Burned {
				return fmt.Errorf("token not burned, ID='%X'", tok.ID)
			}
			burnTxs = append(burnTxs, tx.tx)
			proofs = append(proofs, tok.Proof.Proof)
		}

		joinAttrs := &tokens.JoinFungibleTokenAttributes{
			BurnTransactions:             burnTxs,
			Proofs:                       proofs,
			Backlink:                     targetToken.Backlink,
			InvariantPredicateSignatures: nil,
		}
		sub, err := w.sendTx(targetToken.ID, joinAttrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
			signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, gtx)
			if err != nil {
				return err
			}
			joinAttrs.SetInvariantPredicateSignatures(signatures)
			return anypb.MarshalFrom(tx.TransactionAttributes, joinAttrs, proto.MarshalOptions{})
		})
		if err != nil {
			return err
		}
		err = w.syncToUnit(ctx, sub)
		if err != nil {
			return err
		}
	}
	return nil
}
