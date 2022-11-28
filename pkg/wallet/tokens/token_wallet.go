package tokens

import (
	"context"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
)

const (
	uriMaxSize  = 4 * 1024
	dataMaxSize = 64 * 1024
)

var (
	ErrInvalidBlockSystemID = errors.New("invalid system identifier")
	ErrAttributesMissing    = errors.New("attributes missing")
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

func (w *Wallet) GetAccountManager() wallet.AccountManager {
	return w.mw
}

func (w *Wallet) Shutdown() {
	w.mw.Shutdown()
	if w.db != nil {
		w.db.Close()
	}
}

func (w *Wallet) NewFungibleType(ctx context.Context, attrs *tokens.CreateFungibleTokenTypeAttributes, typeId TokenTypeID, subtypePredicateArgs []*CreationInput) (TokenID, error) {
	log.Info("Creating new fungible token type")
	return w.newType(ctx, attrs, typeId, subtypePredicateArgs)
}

func (w *Wallet) NewNonFungibleType(ctx context.Context, attrs *tokens.CreateNonFungibleTokenTypeAttributes, typeId TokenTypeID, subtypePredicateArgs []*CreationInput) (TokenID, error) {
	log.Info("Creating new NFT type")
	return w.newType(ctx, attrs, typeId, subtypePredicateArgs)
}

func (w *Wallet) NewFungibleToken(ctx context.Context, accNr uint64, attrs *tokens.MintFungibleTokenAttributes, mintPredicateArgs []*CreationInput) (TokenID, error) {
	log.Info("Creating new fungible token")
	return w.newToken(ctx, accNr, attrs, nil, mintPredicateArgs)
}

func (w *Wallet) NewNFT(ctx context.Context, accNr uint64, attrs *tokens.MintNonFungibleTokenAttributes, tokenId TokenID, mintPredicateArgs []*CreationInput) (TokenID, error) {
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

func (w *Wallet) GetTokenType(ctx context.Context, typeId TokenTypeID) (*TokenUnitType, error) {
	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}
	return w.db.Do().GetTokenType(typeId)
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

func (w *Wallet) Transfer(ctx context.Context, accountNumber uint64, tokenId TokenID, receiverPubKey PublicKey) error {
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
	return w.transfer(ctx, acc, t, receiverPubKey)
}

func (w *Wallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenId TokenID, receiverPubKey PublicKey) error {
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

	sub, err := w.sendTx(tokenId, newNonFungibleTransferTxAttrs(t, receiverPubKey), acc, nil)
	if err != nil {
		return err
	}

	return w.syncToUnit(ctx, tokenId, sub.timeout)
}

func (w *Wallet) SendFungible(ctx context.Context, accountNumber uint64, typeId TokenTypeID, targetAmount uint64, receiverPubKey []byte) error {
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
	var submissions map[string]*submittedTx
	var maxTimeout uint64
	// optimization: first try to make a single operation instead of iterating through all tokens in doSendMultiple
	if closestMatch.Amount >= targetAmount {
		var sub *submittedTx
		sub, err = w.sendSplitOrTransferTx(acc, targetAmount, closestMatch, receiverPubKey)
		submissions = make(map[string]*submittedTx, 1)
		submissions[sub.id.String()] = sub
		maxTimeout = sub.timeout
	} else {
		submissions, maxTimeout, err = w.doSendMultiple(targetAmount, fungibleTokens, acc, receiverPubKey)
	}

	// error might have happened, but some submissions could have succeeded
	syncErr := w.syncToUnits(ctx, submissions, maxTimeout)

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
