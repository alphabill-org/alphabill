package tokens

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	uriMaxSize  = 4 * 1024
	dataMaxSize = 64 * 1024
	nameMaxSize = 256
)

var (
	errInvalidURILength  = fmt.Errorf("URI exceeds the maximum allowed size of %v bytes", uriMaxSize)
	errInvalidDataLength = fmt.Errorf("data exceeds the maximum allowed size of %v bytes", dataMaxSize)
	errInvalidNameLength = fmt.Errorf("name exceeds the maximum allowed size of %v bytes", nameMaxSize)

	ErrNoFeeCredit           = errors.New("no fee credit in token wallet")
	ErrInsufficientFeeCredit = errors.New("insufficient fee credit balance for transaction(s)")
)

type (
	Wallet struct {
		systemID   []byte
		txs        block.TxConverter
		am         account.Manager
		backend    TokenBackend
		confirmTx  bool
		feeManager *fees.FeeManager
	}

	TokenBackend interface {
		GetToken(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error)
		GetTokens(ctx context.Context, kind backend.Kind, owner wallet.PubKey, offsetKey string, limit int) ([]backend.TokenUnit, string, error)
		GetTokenTypes(ctx context.Context, kind backend.Kind, creator wallet.PubKey, offsetKey string, limit int) ([]backend.TokenUnitType, string, error)
		GetTypeHierarchy(ctx context.Context, id backend.TokenTypeID) ([]backend.TokenUnitType, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error
		GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
		GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error)
	}

	MoneyDataProvider interface {
		SystemID() []byte
		fees.TxPublisher
	}
)

func New(systemID []byte, backendUrl string, am account.Manager, confirmTx bool, feeManager *fees.FeeManager) (*Wallet, error) {
	if !strings.HasPrefix(backendUrl, "http://") && !strings.HasPrefix(backendUrl, "https://") {
		backendUrl = "http://" + backendUrl
	}
	addr, err := url.Parse(backendUrl)
	if err != nil {
		return nil, err
	}
	txs, err := tokens.New(
		tokens.WithSystemIdentifier(systemID),
		tokens.WithTrustBase(map[string]abcrypto.Verifier{"test": nil}),
	)
	if err != nil {
		return nil, err
	}
	return &Wallet{
		systemID:   systemID,
		am:         am,
		txs:        txs,
		backend:    client.New(*addr),
		confirmTx:  confirmTx,
		feeManager: feeManager,
	}, nil
}

func (w *Wallet) Shutdown() {
	w.am.Close()
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) NewFungibleType(ctx context.Context, accNr uint64, attrs CreateFungibleTokenTypeAttributes, typeId backend.TokenTypeID, subtypePredicateArgs []*PredicateInput) (backend.TokenTypeID, error) {
	log.Info("Creating new fungible token type")
	if attrs.ParentTypeId != nil && !bytes.Equal(attrs.ParentTypeId, backend.NoParent) {
		parentType, err := w.GetTokenType(ctx, attrs.ParentTypeId)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent type: %w", err)
		}
		if parentType.DecimalPlaces != attrs.DecimalPlaces {
			return nil, fmt.Errorf("parent type requires %d decimal places, got %d", parentType.DecimalPlaces, attrs.DecimalPlaces)
		}
	}
	return w.newType(ctx, accNr, attrs.toProtobuf(), typeId, subtypePredicateArgs)
}

func (w *Wallet) NewNonFungibleType(ctx context.Context, accNr uint64, attrs CreateNonFungibleTokenTypeAttributes, typeId backend.TokenTypeID, subtypePredicateArgs []*PredicateInput) (backend.TokenTypeID, error) {
	log.Info("Creating new NFT type")
	return w.newType(ctx, accNr, attrs.toProtobuf(), typeId, subtypePredicateArgs)
}

func (w *Wallet) NewFungibleToken(ctx context.Context, accNr uint64, typeId backend.TokenTypeID, amount uint64, bearerPredicate wallet.Predicate, mintPredicateArgs []*PredicateInput) (backend.TokenID, error) {
	log.Info("Creating new fungible token")
	attrs := &tokens.MintFungibleTokenAttributes{
		Bearer:                           bearerPredicate,
		TypeID:                           typeId,
		Value:                            amount,
		TokenCreationPredicateSignatures: nil,
	}
	return w.newToken(ctx, accNr, attrs, nil, mintPredicateArgs)
}

func (w *Wallet) NewNFT(ctx context.Context, accNr uint64, attrs MintNonFungibleTokenAttributes, tokenId backend.TokenID, mintPredicateArgs []*PredicateInput) (backend.TokenID, error) {
	log.Info("Creating new NFT")
	if len(attrs.Name) > nameMaxSize {
		return nil, errInvalidNameLength
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
	return w.newToken(ctx, accNr, attrs.toProtobuf(), tokenId, mintPredicateArgs)
}

func (w *Wallet) ListTokenTypes(ctx context.Context, accountNumber uint64, kind backend.Kind) ([]*backend.TokenUnitType, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}
	allTokenTypes := make([]*backend.TokenUnitType, 0)
	fetchForPubKey := func(pubKey []byte) ([]*backend.TokenUnitType, error) {
		allTokenTypesForKey := make([]*backend.TokenUnitType, 0)
		var types []backend.TokenUnitType
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

	for _, key := range keys {
		types, err := fetchForPubKey(key.PubKey)
		if err != nil {
			return nil, err
		}
		allTokenTypes = append(allTokenTypes, types...)
	}

	return allTokenTypes, nil
}

// GetTokenType returns non-nil TokenUnitType or error if not found or other issues
func (w *Wallet) GetTokenType(ctx context.Context, typeId backend.TokenTypeID) (*backend.TokenUnitType, error) {
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
func (w *Wallet) ListTokens(ctx context.Context, kind backend.Kind, accountNumber uint64) (map[uint64][]*backend.TokenUnit, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}

	// account number -> list of its tokens
	allTokensByAccountNumber := make(map[uint64][]*backend.TokenUnit, len(keys))
	for _, key := range keys {
		ts, err := w.getTokens(ctx, kind, key.PubKey)
		if err != nil {
			return nil, err
		}
		allTokensByAccountNumber[key.idx+1] = ts
	}

	return allTokensByAccountNumber, nil
}

type accountKey struct {
	*account.AccountKey
	idx uint64
}

func (w *Wallet) getAccounts(accountNumber uint64) ([]*accountKey, error) {
	if accountNumber > AllAccounts {
		key, err := w.am.GetAccountKey(accountNumber - 1)
		if err != nil {
			return nil, err
		}
		return []*accountKey{{AccountKey: key, idx: accountNumber - 1}}, nil
	}
	keys, err := w.am.GetAccountKeys()
	if err != nil {
		return nil, err
	}
	wrappers := make([]*accountKey, len(keys))
	for i := range keys {
		wrappers[i] = &accountKey{AccountKey: keys[i], idx: uint64(i)}
	}
	return wrappers, nil
}

func (w *Wallet) getTokens(ctx context.Context, kind backend.Kind, pubKey wallet.PubKey) ([]*backend.TokenUnit, error) {
	allTokens := make([]*backend.TokenUnit, 0)
	var ts []backend.TokenUnit
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

func (w *Wallet) GetToken(ctx context.Context, owner wallet.PubKey, kind backend.Kind, tokenId backend.TokenID) (*backend.TokenUnit, error) {
	token, err := w.backend.GetToken(ctx, tokenId)
	if err != nil {
		return nil, fmt.Errorf("error fetching token %X: %w", tokenId, err)
	}
	return token, nil
}

func (w *Wallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenId backend.TokenID, receiverPubKey wallet.PubKey, invariantPredicateArgs []*PredicateInput) error {
	key, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return err
	}
	err = w.ensureFeeCredit(ctx, key, 1)
	if err != nil {
		return err
	}
	token, err := w.GetToken(ctx, key.PubKey, backend.NonFungible, tokenId)
	if err != nil {
		return err
	}
	attrs := newNonFungibleTransferTxAttrs(token, receiverPubKey)
	sub, err := w.prepareTxSubmission(ctx, wallet.UnitID(tokenId), attrs, key, w.GetRoundNumber, func(tx *types.TransactionOrder, gtx txsystem.GenericTransaction) error {
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
	return sub.ToBatch(w.backend, key.PubKey).SendTx(ctx, w.confirmTx)
}

func (w *Wallet) SendFungible(ctx context.Context, accountNumber uint64, typeId backend.TokenTypeID, targetAmount uint64, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) error {
	if accountNumber < 1 {
		return fmt.Errorf("invalid account number: %d", accountNumber)
	}
	acc, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return err
	}
	err = w.ensureFeeCredit(ctx, acc, 1)
	if err != nil {
		return err
	}
	tokensByAcc, err := w.ListTokens(ctx, backend.Fungible, accountNumber)
	if err != nil {
		return err
	}
	tokens, found := tokensByAcc[accountNumber]
	if !found {
		return fmt.Errorf("account %d has no tokens", accountNumber)
	}
	var matchingTokens []*backend.TokenUnit
	var totalBalance uint64
	// find the best unit candidate for transfer or split, value must be equal or larger than the target amount
	var closestMatch *backend.TokenUnit
	for _, token := range tokens {
		if token.Kind != backend.Fungible {
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
		sub, err := w.prepareSplitOrTransferTx(ctx, acc, targetAmount, closestMatch, receiverPubKey, invariantPredicateArgs, w.GetRoundNumber)
		if err != nil {
			return err
		}
		return sub.ToBatch(w.backend, acc.PubKey).SendTx(ctx, w.confirmTx)
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
	err = w.ensureFeeCredit(ctx, acc, 1)
	if err != nil {
		return err
	}
	t, err := w.GetToken(ctx, acc.PubKey, backend.NonFungible, tokenId)
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

	sub, err := w.prepareTxSubmission(ctx, tokenId, attrs, acc, w.GetRoundNumber, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
	return sub.ToBatch(w.backend, acc.PubKey).SendTx(ctx, w.confirmTx)
}

// GetFeeCreditBill returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCreditBill(ctx context.Context, cmd fees.GetFeeCreditCmd) (*bp.Bill, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	return w.FetchFeeCreditBill(ctx, accountKey.PrivKeyHash)
}

// FetchFeeCreditBill returns fee credit bill for given unitID
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) FetchFeeCreditBill(ctx context.Context, unitID []byte) (*bp.Bill, error) {
	fcb, err := w.backend.GetFeeCreditBill(ctx, unitID)
	if err != nil {
		return nil, err
	}
	if fcb == nil {
		return nil, nil
	}
	return &bp.Bill{
		Id:            fcb.Id,
		Value:         fcb.Value,
		TxHash:        fcb.TxHash,
		FcBlockNumber: fcb.FCBlockNumber,
	}, nil
}

func (w *Wallet) GetRoundNumber(ctx context.Context) (uint64, error) {
	return w.backend.GetRoundNumber(ctx)
}

func (w *Wallet) AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) ([]*block.TxProof, error) {
	return w.feeManager.AddFeeCredit(ctx, cmd)
}

func (w *Wallet) ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) ([]*block.TxProof, error) {
	return w.feeManager.ReclaimFeeCredit(ctx, cmd)
}

func (w *Wallet) ensureFeeCredit(ctx context.Context, accountKey *account.AccountKey, txCount int) error {
	fcb, err := w.FetchFeeCreditBill(ctx, accountKey.PrivKeyHash)
	if err != nil {
		return err
	}
	if fcb == nil {
		return ErrNoFeeCredit
	}
	maxFee := uint64(txCount) * tx_builder.MaxFee
	if fcb.GetValue() < maxFee {
		return ErrInsufficientFeeCredit
	}
	return nil
}
