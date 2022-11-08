package tokens

import (
	"context"
	"errors"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
)

const (
	txTimeoutBlockCount     = 100
	AllAccounts         int = -1
)

var (
	ErrInvalidBlockSystemID = errors.New("invalid system identifier")
)

type (
	TokensWallet struct {
		mw            *money.Wallet
		db            *tokensDb
		txs           block.TxConverter
		waitTx        bool
		blockListener wallet.BlockProcessor
	}

	PublicKey []byte
)

func Load(mw *money.Wallet, waitTx bool) (*TokensWallet, error) {
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
	w := &TokensWallet{mw, db, txs, waitTx, nil}
	w.mw.Wallet = wallet.New().
		SetBlockProcessor(w).
		SetABClient(mw.AlphabillClient).
		Build()
	return w, nil
}

func (w *TokensWallet) Shutdown() {
	w.mw.Shutdown()
}

func (w *TokensWallet) NewFungibleType(ctx context.Context, attrs *tokens.CreateFungibleTokenTypeAttributes, typeId TokenTypeId) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new fungible token type"))
	return w.newType(ctx, attrs, typeId)
}

func (w *TokensWallet) NewNonFungibleType(ctx context.Context, attrs *tokens.CreateNonFungibleTokenTypeAttributes, typeId TokenTypeId) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new NFT type"))
	return w.newType(ctx, attrs, typeId)
}

func (w *TokensWallet) NewFungibleToken(ctx context.Context, accNr uint64, attrs *tokens.MintFungibleTokenAttributes) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new fungible token"))
	return w.newToken(ctx, accNr, attrs, nil)
}

func (w *TokensWallet) NewNFT(ctx context.Context, accNr uint64, attrs *tokens.MintNonFungibleTokenAttributes, tokenId TokenId) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new NFT"))
	return w.newToken(ctx, accNr, attrs, tokenId)
}

func (w *TokensWallet) ListTokenTypes(ctx context.Context) ([]string, error) {
	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}

	types, err := w.db.Do().GetTokenTypes()
	if err != nil {
		return nil, err
	}
	res := make([]string, len(types))
	for _, t := range types {
		m := fmt.Sprintf("Id=%X, symbol=%s, kind: %s", t.Id, t.Symbol, t.Kind.pretty())
		log.Info(m)
		res = append(res, m)
	}
	return res, nil
}

func (w *TokensWallet) ListTokens(ctx context.Context, kind TokenKind, accountNumber int) ([]string, error) {

	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}

	var pubKeys [][]byte
	if accountNumber > AllAccounts+1 {
		pubKeys[0], err = w.mw.GetPublicKey(uint64(accountNumber - 1))
		if err != nil {
			return nil, err
		}
	} else {
		pubKeys, err = w.mw.GetPublicKeys()
		if err != nil {
			return nil, err
		}
	}

	res := make([]string, len(pubKeys)+1)
	for n := 0; n <= len(pubKeys); n++ {
		tokens, err := w.db.Do().GetTokens(uint64(n))
		if err != nil {
			return nil, err
		}
		if len(tokens) > 0 {
			// TODO filter by kind
			var m string
			if n == alwaysTrueTokensAccountNumber {
				m = fmt.Sprintf("Tokens spendable by anyone: ")
			} else {
				m = fmt.Sprintf("Account #%v (key '%X') tokens: ", n, pubKeys[n-1])
			}
			log.Info(m)
			res = append(res, m)
			for _, token := range tokens {
				m = fmt.Sprintf("Id=%X, symbol=%s, value=%v, kind: %s", token.Id, token.Symbol, token.Amount, token.Kind.pretty())
				log.Info(m)
				res = append(res, m)
			}
		}
	}
	return res, nil
}

func (w *TokensWallet) Transfer(ctx context.Context, accountNumber uint64, tokenId TokenId, receiverPubKey PublicKey) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	t, found, err := w.db.Do().GetToken(accountNumber, tokenId)
	if err != nil {
		return err
	}
	if !found {
		return errors.New(fmt.Sprintf("token with id=%X not found under account #%v", tokenId, accountNumber))
	}
	return w.transfer(ctx, acc, t, receiverPubKey)
}

func (w *TokensWallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenId TokenId, receiverPubKey PublicKey) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	t, found, err := w.db.Do().GetToken(accountNumber, tokenId)
	if err != nil {
		return err
	}
	if !found {
		return errors.New(fmt.Sprintf("token with id=%X not found under account #%v", tokenId, accountNumber))
	}

	sub, err := w.sendTx(tokenId, newNonFungibleTransferTxAttrs(t, receiverPubKey), acc)
	if err != nil {
		return err
	}

	return w.syncToUnit(ctx, tokenId, sub.timeout)
}

func (w *TokensWallet) SendFungible(ctx context.Context, accountNumber uint64, typeId TokenTypeId, targetAmount uint64, receiverPubKey []byte) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	tokens, err := w.db.Do().GetTokens(accountNumber)
	if err != nil {
		return err
	}
	fungibleTokens := make([]*token, 0)
	var totalBalance uint64 = 0
	// find the best unit candidate for transfer or split, value must be equal or larger than the target amount
	var closestMatch *token = nil
	for _, token := range tokens {
		if token.isFungible() && typeId.equal(token.TypeId) {
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
		return errors.New(fmt.Sprintf("insufficient value: got %v, need %v", totalBalance, targetAmount))
	}
	var submissions map[string]*submittedTx
	var maxTimeout uint64
	// optimization: first try to make a single operation instead of iterating through all tokens in doSendMultiple
	if closestMatch.Amount >= targetAmount {
		var sub *submittedTx
		sub, err = w.sendSplitOrTransferTx(acc, targetAmount, closestMatch, receiverPubKey)
		submissions = make(map[string]*submittedTx, 1)
		submissions[sub.id.string()] = sub
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

func (w *TokensWallet) getAccountKey(accountNumber uint64) (*wallet.AccountKey, error) {
	if accountNumber > 0 {
		return w.mw.GetAccountKey(accountNumber - 1)
	}
	return nil, nil
}
