package tokens

import (
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"net/url"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
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
		systemID  []byte
		txs       block.TxConverter
		am        account.Manager
		backend   TokenBackend
		confirmTx bool // TODO: ?
	}

	TokenBackend interface {
		GetTokens(ctx context.Context, kind twb.Kind, owner twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnit, string, error)
		GetTokenTypes(ctx context.Context, kind twb.Kind, creator twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnitType, string, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		PostTransactions(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error
	}
)

func Load(backendUrl string, am account.Manager) (*Wallet, error) {
	addr, err := url.Parse(backendUrl)
	if err != nil {
		return nil, err
	}
	systemID := tokens.DefaultTokenTxSystemIdentifier
	txs, err := tokens.New(tokens.WithSystemIdentifier(systemID))
	if err != nil {
		return nil, err
	}
	w := &Wallet{systemID: systemID, am: am, txs: txs, backend: client.New(*addr)}

	return w, nil
}

func (w *Wallet) Shutdown() {
	w.am.Close()
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) NewFungibleType(ctx context.Context, accNr uint64, attrs *tokens.CreateFungibleTokenTypeAttributes, typeId twb.TokenTypeID, subtypePredicateArgs []*PredicateInput) (twb.TokenID, error) {
	log.Info("Creating new fungible token type")
	// TODO check if parent type's decimal places match
	return w.newType(ctx, accNr, attrs, typeId, subtypePredicateArgs)
}

func (w *Wallet) NewNonFungibleType(ctx context.Context, accNr uint64, attrs *tokens.CreateNonFungibleTokenTypeAttributes, typeId twb.TokenTypeID, subtypePredicateArgs []*PredicateInput) (twb.TokenID, error) {
	log.Info("Creating new NFT type")
	return w.newType(ctx, accNr, attrs, typeId, subtypePredicateArgs)
}

func (w *Wallet) NewFungibleToken(ctx context.Context, accNr uint64, attrs *tokens.MintFungibleTokenAttributes, mintPredicateArgs []*PredicateInput) (twb.TokenID, error) {
	log.Info("Creating new fungible token")
	return w.newToken(ctx, accNr, attrs, nil, mintPredicateArgs)
}

func (w *Wallet) NewNFT(ctx context.Context, accNr uint64, attrs *tokens.MintNonFungibleTokenAttributes, tokenId twb.TokenID, mintPredicateArgs []*PredicateInput) (twb.TokenID, error) {
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

func (w *Wallet) ListTokenTypes(ctx context.Context, kind twb.Kind) ([]*twb.TokenUnitType, error) {
	pubKeys, err := w.am.GetPublicKeys()
	if err != nil {
		return nil, err
	}
	allTokenTypes := make([]*twb.TokenUnitType, 0)
	fetchForPubKey := func(pubKey []byte) ([]*twb.TokenUnitType, error) {
		allTokenTypesForKey := make([]*twb.TokenUnitType, 0)
		var types []twb.TokenUnitType
		offsetKey := ""
		var err error
		for {
			types, offsetKey, err = w.backend.GetTokenTypes(ctx, kind, pubKey, offsetKey, 0)
			if err != nil {
				return nil, err
			}
			for i := range types {
				allTokenTypesForKey = append(allTokenTypesForKey, &types[i])
			}
			if offsetKey == "" {
				break
			}
		}
		return allTokenTypesForKey, nil
	}

	for _, pubKey := range pubKeys {
		types, err := fetchForPubKey(pubKey)
		if err != nil {
			return nil, err
		}
		allTokenTypes = append(allTokenTypes, types...)
	}

	return allTokenTypes, nil
}

// GetTokenType returns non-nil TokenUnitType or error if not found or other issues
func (w *Wallet) GetTokenType(ctx context.Context, typeId twb.TokenTypeID) (*twb.TokenUnitType, error) {
	offsetKey := ""
	var err error
	for {
		var types []twb.TokenUnitType
		// TODO: allow passing type id to filter specific unit on the backend
		types, offsetKey, err = w.backend.GetTokenTypes(ctx, twb.Any, nil, offsetKey, 0)
		if err != nil {
			return nil, err
		}
		for _, tokenType := range types {
			if bytes.Equal(tokenType.ID, typeId) {
				return &tokenType, nil
			}
		}
		if offsetKey == "" {
			break
		}
	}
	return nil, fmt.Errorf("error token type %X not found", typeId)
}

// ListTokens specify accountNumber=0 to list tokens from all accounts
func (w *Wallet) ListTokens(ctx context.Context, kind twb.Kind, accountNumber uint64) (map[uint64][]*twb.TokenUnit, error) {
	var pubKeys [][]byte
	var err error
	singleKey := accountNumber > AllAccounts
	accountIdx := accountNumber - 1
	if singleKey {
		key, err := w.am.GetPublicKey(accountIdx)
		if err != nil {
			return nil, err
		}
		pubKeys = append(pubKeys, key)
	} else {
		pubKeys, err = w.am.GetPublicKeys()
		if err != nil {
			return nil, err
		}
	}

	// account number -> list of its tokens
	allTokensByAccountNumber := make(map[uint64][]*twb.TokenUnit, 0)
	fetchForPubKey := func(pubKey []byte) ([]*twb.TokenUnit, error) {
		allTokensForKey := make([]*twb.TokenUnit, 0)
		var ts []twb.TokenUnit
		offsetKey := ""
		var err error
		for {
			ts, offsetKey, err = w.backend.GetTokens(ctx, kind, pubKey, offsetKey, 0)
			if err != nil {
				return nil, err
			}
			for i := range ts {
				allTokensForKey = append(allTokensForKey, &ts[i])
			}
			if offsetKey == "" {
				break
			}
		}
		return allTokensForKey, nil
	}

	if singleKey {
		ts, err := fetchForPubKey(pubKeys[0])
		if err != nil {
			return nil, err
		}
		allTokensByAccountNumber[accountNumber] = ts
		return allTokensByAccountNumber, nil
	}

	for idx, pubKey := range pubKeys {
		ts, err := fetchForPubKey(pubKey)
		if err != nil {
			return nil, err
		}
		allTokensByAccountNumber[uint64(idx)+1] = ts
	}
	return allTokensByAccountNumber, nil
}

func (w *Wallet) GetToken(ctx context.Context, owner twb.PubKey, tokenId twb.TokenID) (*twb.TokenUnit, error) {
	offsetKey := ""
	var err error
	for {
		var tokens []twb.TokenUnit
		// TODO: allow passing type id to filter specific unit on the backend
		tokens, offsetKey, err = w.backend.GetTokens(ctx, twb.Any, owner, offsetKey, 0)
		if err != nil {
			return nil, err
		}
		for _, token := range tokens {
			if bytes.Equal(token.ID, tokenId) {
				return &token, nil
			}
		}
		if offsetKey == "" {
			break
		}
	}
	return nil, fmt.Errorf("error token %X not found", tokenId)
}

func (w *Wallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenId twb.TokenID, receiverPubKey twb.PubKey, invariantPredicateArgs []*PredicateInput) error {
	key, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return err
	}

	token, err := w.GetToken(ctx, key.PubKey, tokenId)
	if err != nil {
		return err
	}
	attrs := newNonFungibleTransferTxAttrs(token, receiverPubKey)
	_, err = w.sendTx(ctx, tokenId, attrs, key, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.am, invariantPredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.InvariantPredicateSignatures = signatures
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})

	return err
}

func (w *Wallet) SendFungible(ctx context.Context, accountNumber uint64, typeId twb.TokenTypeID, targetAmount uint64, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) error {
	if accountNumber < 1 {
		return fmt.Errorf("invalid account number: %d", accountNumber)
	}
	acc, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return err
	}
	tokensByAcc, err := w.ListTokens(ctx, twb.Fungible, accountNumber)
	if err != nil {
		return err
	}
	tokens, found := tokensByAcc[accountNumber]
	if !found {
		return fmt.Errorf("account %d has no tokens", accountNumber)
	}
	matchingTokens := make([]*twb.TokenUnit, 0)
	var totalBalance uint64 = 0
	// find the best unit candidate for transfer or split, value must be equal or larger than the target amount
	var closestMatch *twb.TokenUnit = nil
	for _, token := range tokens {
		if token.Kind != twb.Fungible {
			return fmt.Errorf("expected fungible token, got %v, token %X", token.Kind.String(), token.ID)
		}
		if typeId.Equal(token.TypeID) {
			matchingTokens = append(matchingTokens, token)
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
	// optimization: first try to make a single operation instead of iterating through all tokens in doSendMultiple
	if closestMatch.Amount >= targetAmount {
		var sub *submittedTx
		sub, err = w.sendSplitOrTransferTx(ctx, acc, targetAmount, closestMatch, receiverPubKey, invariantPredicateArgs)
		submissions = make(map[string]*submittedTx, 1)
		submissions[sub.id.String()] = sub
	} else {
		submissions, _, err = w.doSendMultiple(ctx, targetAmount, matchingTokens, acc, receiverPubKey, invariantPredicateArgs)
	}

	return err
}

func (w *Wallet) UpdateNFTData(ctx context.Context, accountNumber uint64, tokenId []byte, data []byte, updatePredicateArgs []*PredicateInput) error {
	panic("not implemented")
}

func (w *Wallet) getRoundNumber(ctx context.Context) (uint64, error) {
	return w.backend.GetRoundNumber(ctx)
}
