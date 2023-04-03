package tokens

import (
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"net/url"
	"strings"

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
	errAttributesMissing = goerrors.New("attributes missing")
	errInvalidURILength  = fmt.Errorf("URI exceeds the maximum allowed size of %v bytes", uriMaxSize)
	errInvalidDataLength = fmt.Errorf("data exceeds the maximum allowed size of %v bytes", dataMaxSize)
)

type (
	Wallet struct {
		systemID  []byte
		txs       block.TxConverter
		am        account.Manager
		backend   TokenBackend
		confirmTx bool
	}

	TokenBackend interface {
		GetToken(ctx context.Context, id twb.TokenID) (*twb.TokenUnit, error)
		GetTokens(ctx context.Context, kind twb.Kind, owner twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnit, string, error)
		GetTokenTypes(ctx context.Context, kind twb.Kind, creator twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnitType, string, error)
		GetTypeHierarchy(ctx context.Context, id twb.TokenTypeID) ([]twb.TokenUnitType, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		PostTransactions(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error
		GetTxProof(ctx context.Context, unitID twb.UnitID, txHash twb.TxHash) (*twb.Proof, error)
	}
)

func New(systemID []byte, backendUrl string, am account.Manager, confirmTx bool) (*Wallet, error) {
	if !strings.HasPrefix(backendUrl, "http://") && !strings.HasPrefix(backendUrl, "https://") {
		backendUrl = "http://" + backendUrl
	}
	addr, err := url.Parse(backendUrl)
	if err != nil {
		return nil, err
	}
	txs, err := tokens.New(tokens.WithSystemIdentifier(systemID))
	if err != nil {
		return nil, err
	}
	w := &Wallet{systemID: systemID, am: am, txs: txs, backend: client.New(*addr), confirmTx: confirmTx}

	return w, nil
}

func (w *Wallet) Shutdown() {
	w.am.Close()
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) NewFungibleType(ctx context.Context, accNr uint64, attrs *tokens.CreateFungibleTokenTypeAttributes, typeId twb.TokenTypeID, subtypePredicateArgs []*PredicateInput) (twb.TokenTypeID, error) {
	log.Info("Creating new fungible token type")
	if attrs.ParentTypeId != nil && !bytes.Equal(attrs.ParentTypeId, twb.NoParent) {
		parentType, err := w.GetTokenType(ctx, attrs.ParentTypeId)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent type: %w", err)
		}
		if parentType.DecimalPlaces != attrs.DecimalPlaces {
			return nil, fmt.Errorf("parent type requires %d decimal places, got %d", parentType.DecimalPlaces, attrs.DecimalPlaces)
		}
	}
	return w.newType(ctx, accNr, attrs, typeId, subtypePredicateArgs)
}

func (w *Wallet) NewNonFungibleType(ctx context.Context, accNr uint64, attrs *tokens.CreateNonFungibleTokenTypeAttributes, typeId twb.TokenTypeID, subtypePredicateArgs []*PredicateInput) (twb.TokenTypeID, error) {
	log.Info("Creating new NFT type")
	return w.newType(ctx, accNr, attrs, typeId, subtypePredicateArgs)
}

func (w *Wallet) NewFungibleToken(ctx context.Context, accNr uint64, typeId twb.TokenTypeID, amount uint64, mintPredicateArgs []*PredicateInput) (twb.TokenID, error) {
	log.Info("Creating new fungible token")
	attrs := &tokens.MintFungibleTokenAttributes{
		Bearer:                           nil,
		Type:                             typeId,
		Value:                            amount,
		TokenCreationPredicateSignatures: nil,
	}
	return w.newToken(ctx, accNr, attrs, nil, mintPredicateArgs)
}

func (w *Wallet) NewNFT(ctx context.Context, accNr uint64, attrs *tokens.MintNonFungibleTokenAttributes, tokenId twb.TokenID, mintPredicateArgs []*PredicateInput) (twb.TokenID, error) {
	log.Info("Creating new NFT")
	if attrs == nil {
		return nil, errAttributesMissing
	}
	if len(attrs.Uri) > uriMaxSize {
		return nil, errInvalidURILength
	}
	if attrs.Uri != "" && !util.IsValidURI(attrs.Uri) {
		return nil, fmt.Errorf("URI '%s' is invalid", attrs.Uri)
	}
	if len(attrs.Data) > dataMaxSize {
		return nil, errInvalidDataLength
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
	types, err := w.backend.GetTypeHierarchy(ctx, typeId)
	if err != nil {
		return nil, err
	}
	for i := range types {
		if bytes.Equal(types[i].ID, typeId) {
			return &types[i], nil
		}
	}
	return nil, fmt.Errorf("token type %X not found", typeId)
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

	if singleKey {
		ts, err := w.getTokens(ctx, kind, pubKeys[0])
		if err != nil {
			return nil, err
		}
		allTokensByAccountNumber[accountNumber] = ts
		return allTokensByAccountNumber, nil
	}

	for idx, pubKey := range pubKeys {
		ts, err := w.getTokens(ctx, kind, pubKey)
		if err != nil {
			return nil, err
		}
		allTokensByAccountNumber[uint64(idx)+1] = ts
	}
	return allTokensByAccountNumber, nil
}

func (w *Wallet) getTokens(ctx context.Context, kind twb.Kind, pubKey twb.PubKey) ([]*twb.TokenUnit, error) {
	allTokens := make([]*twb.TokenUnit, 0)
	var ts []twb.TokenUnit
	offsetKey := ""
	var err error
	for {
		ts, offsetKey, err = w.backend.GetTokens(ctx, kind, pubKey, offsetKey, 0)
		if err != nil {
			return nil, err
		}
		for i := range ts {
			allTokens = append(allTokens, &ts[i])
		}
		if offsetKey == "" {
			break
		}
	}
	return allTokens, nil
}

func (w *Wallet) GetToken(ctx context.Context, owner twb.PubKey, kind twb.Kind, tokenId twb.TokenID) (*twb.TokenUnit, error) {
	token, err := w.backend.GetToken(ctx, tokenId)
	if err != nil {
		return nil, fmt.Errorf("error fetching token %X: %w", tokenId, err)
	}
	return token, nil
}

func (w *Wallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenId twb.TokenID, receiverPubKey twb.PubKey, invariantPredicateArgs []*PredicateInput) error {
	key, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return err
	}

	token, err := w.GetToken(ctx, key.PubKey, twb.NonFungible, tokenId)
	if err != nil {
		return err
	}
	attrs := newNonFungibleTransferTxAttrs(token, receiverPubKey)
	sub, err := w.prepareTx(ctx, twb.UnitID(tokenId), attrs, key, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.am, invariantPredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.InvariantPredicateSignatures = signatures
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return err
	}
	return sub.toBatch(w.backend, key.PubKey).sendTx(ctx, w.confirmTx)
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
	var matchingTokens []*twb.TokenUnit
	var totalBalance uint64
	// find the best unit candidate for transfer or split, value must be equal or larger than the target amount
	var closestMatch *twb.TokenUnit
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
	// optimization: first try to make a single operation instead of iterating through all tokens in doSendMultiple
	if closestMatch.Amount >= targetAmount {
		sub, err := w.prepareSplitOrTransferTx(ctx, acc, targetAmount, closestMatch, receiverPubKey, invariantPredicateArgs)
		if err != nil {
			return err
		}
		return sub.toBatch(w.backend, acc.PubKey).sendTx(ctx, w.confirmTx)
	} else {
		return w.doSendMultiple(ctx, targetAmount, matchingTokens, acc, receiverPubKey, invariantPredicateArgs)
	}
}

func (w *Wallet) UpdateNFTData(ctx context.Context, accountNumber uint64, tokenId []byte, data []byte, updatePredicateArgs []*PredicateInput) error {
	if accountNumber < 1 {
		return fmt.Errorf("invalid account number: %d", accountNumber)
	}
	acc, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return err
	}
	t, err := w.GetToken(ctx, acc.PubKey, twb.NonFungible, tokenId)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("token with id=%X not found under account #%v", tokenId, accountNumber)
	}

	attrs := &tokens.UpdateNonFungibleTokenAttributes{
		Data:                 data,
		Backlink:             t.TxHash,
		DataUpdateSignatures: nil,
	}

	sub, err := w.prepareTx(ctx, tokenId, attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.am, updatePredicateArgs, gtx)
		if err != nil {
			return err
		}
		attrs.DataUpdateSignatures = signatures
		return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	})
	if err != nil {
		return err
	}
	return sub.toBatch(w.backend, acc.PubKey).sendTx(ctx, w.confirmTx)
}

func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64, tokenTypes []twb.TokenTypeID, invariantPredicateArgs []*PredicateInput) error {
	var keys []*account.AccountKey
	var err error
	singleKey := false
	if accountNumber > AllAccounts {
		key, err := w.am.GetAccountKey(accountNumber - 1)
		if err != nil {
			return err
		}
		keys = append(keys, key)
		singleKey = true
	} else {
		keys, err = w.am.GetAccountKeys()
		if err != nil {
			return err
		}
	}
	if singleKey {
		return w.collectDust(ctx, keys[0], tokenTypes, invariantPredicateArgs)
	}
	// TODO: rewrite with goroutines?
	for _, key := range keys {
		err = w.collectDust(ctx, key, tokenTypes, invariantPredicateArgs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) collectDust(ctx context.Context, acc *account.AccountKey, allowedTokenTypes []twb.TokenTypeID, invariantPredicateArgs []*PredicateInput) error {
	tokensByTypes, err := w.getTokensForDC(ctx, acc.PubKey, allowedTokenTypes)
	if err != nil {
		return err
	}

	for _, v := range tokensByTypes {
		// first token to be joined into
		targetToken := v[0]
		burnBatch := &txSubmissionBatch{
			sender:  acc.PubKey,
			backend: w.backend,
		}
		// burn the rest
		for i := 1; i < len(v); i++ {
			token := v[i]
			attrs := newBurnTxAttrs(token, targetToken.TxHash)
			sub, err := w.prepareTx(ctx, twb.UnitID(token.ID), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
			burnBatch.add(sub)
		}
		if err = burnBatch.sendTx(ctx, true); err != nil {
			return err
		}
		burnTxs := make([]*txsystem.Transaction, 0, len(burnBatch.submissions))
		proofs := make([]*block.BlockProof, 0, len(burnBatch.submissions))
		for _, sub := range burnBatch.submissions {
			burnTxs = append(burnTxs, sub.tx)
			proofs = append(proofs, sub.txProof)
		}

		joinAttrs := &tokens.JoinFungibleTokenAttributes{
			BurnTransactions:             burnTxs,
			Proofs:                       proofs,
			Backlink:                     targetToken.TxHash,
			InvariantPredicateSignatures: nil,
		}

		sub, err := w.prepareTx(ctx, twb.UnitID(targetToken.ID), joinAttrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
		if err = sub.toBatch(w.backend, acc.PubKey).sendTx(ctx, true); err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) getTokensForDC(ctx context.Context, key twb.PubKey, allowedTokenTypes []twb.TokenTypeID) (map[string][]*twb.TokenUnit, error) {
	// find tokens to join
	allTokens, err := w.getTokens(ctx, twb.Fungible, key)
	if err != nil {
		return nil, err
	}
	// group tokens by type
	var tokensByTypes = make(map[string][]*twb.TokenUnit, len(allowedTokenTypes))
	for _, tokenType := range allowedTokenTypes {
		tokensByTypes[string(tokenType)] = make([]*twb.TokenUnit, 0)
	}
	for _, tok := range allTokens {
		tokenTypeStr := string(tok.TypeID)
		tokenz, found := tokensByTypes[tokenTypeStr]
		if !found {
			if len(allowedTokenTypes) == 0 {
				tokenz = make([]*twb.TokenUnit, 0, 1)
			} else {
				continue
			}
		}
		tokensByTypes[tokenTypeStr] = append(tokenz, tok)
	}
	for k, v := range tokensByTypes {
		if len(v) < 2 { // not interested if tokens count is less than two
			delete(tokensByTypes, k)
		}
	}
	return tokensByTypes, nil
}

func (w *Wallet) getRoundNumber(ctx context.Context) (uint64, error) {
	return w.backend.GetRoundNumber(ctx)
}
