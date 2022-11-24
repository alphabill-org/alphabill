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

var (
	ErrInvalidBlockSystemID = errors.New("invalid system identifier")
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

func (w *Wallet) NewFungibleToken(ctx context.Context, accNr uint64, attrs *tokens.MintFungibleTokenAttributes) (TokenID, error) {
	log.Info("Creating new fungible token")
	return w.newToken(ctx, accNr, attrs, nil)
}

func (w *Wallet) NewNFT(ctx context.Context, accNr uint64, attrs *tokens.MintNonFungibleTokenAttributes, tokenId TokenID) (TokenID, error) {
	log.Info("Creating new NFT")
	return w.newToken(ctx, accNr, attrs, tokenId)
}

func (w *Wallet) ListTokenTypes(ctx context.Context) ([]*TokenUnitType, error) {
	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}

	return w.db.Do().GetTokenTypes()
}

// ListTokens specify accountNumber=-1 to list tokens from all accounts
func (w *Wallet) ListTokens(ctx context.Context, kind TokenKind, accountNumber int) (map[int][]*TokenUnit, error) {

	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}

	var pubKeys [][]byte
	skipAlwaysTrue := false
	if accountNumber > AllAccounts+1 {
		key, err := w.mw.GetPublicKey(uint64(accountNumber - 1))
		if err != nil {
			return nil, err
		}
		pubKeys = append(pubKeys, key)
		skipAlwaysTrue = true
	} else if accountNumber != alwaysTrueTokensAccountNumber {
		pubKeys, err = w.mw.GetPublicKeys()
		if err != nil {
			return nil, err
		}
	}

	res := make(map[int][]*TokenUnit, 0)
	// NB! n=0 is a special index for always true predicates, thus iteration goes until len, not len-1
	for n := 0; n <= len(pubKeys); n++ {
		if n == alwaysTrueTokensAccountNumber && skipAlwaysTrue {
			continue
		}
		tokens, err := w.db.Do().GetTokens(uint64(n))
		if err != nil {
			return nil, err
		}
		for _, tok := range tokens {
			if kind&Any > 0 || tok.Kind&kind == kind {
				tokens, found := res[n]
				if found {
					res[n] = append(tokens, tok)
				} else {
					res[n] = []*TokenUnit{tok}
				}
			}
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
