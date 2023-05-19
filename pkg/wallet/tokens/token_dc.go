package tokens

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const maxBurnBatchSize = 100

func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64, allowedTokenTypes []twb.TokenTypeID, invariantPredicateArgs []*PredicateInput) error {
	var keys []*account.AccountKey
	var err error
	if accountNumber > AllAccounts {
		key, err := w.am.GetAccountKey(accountNumber - 1)
		if err != nil {
			return err
		}
		keys = append(keys, key)
	} else {
		keys, err = w.am.GetAccountKeys()
		if err != nil {
			return err
		}
	}

	for _, key := range keys {
		tokensByTypes, err := w.getTokensForDC(ctx, key.PubKey, allowedTokenTypes)
		if err != nil {
			return err
		}
		for _, tokens := range tokensByTypes {
			if err = w.collectDust(ctx, key, tokens, invariantPredicateArgs); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Wallet) collectDust(ctx context.Context, acc *account.AccountKey, typedTokens []*twb.TokenUnit, invariantPredicateArgs []*PredicateInput) error {
	if err := w.ensureFeeCredit(ctx, acc, len(typedTokens)); err != nil {
		return err
	}
	// first token to be joined into
	targetTokenID := sdk.UnitID(typedTokens[0].ID)
	targetTokenBacklink := typedTokens[0].TxHash
	burnTokens := typedTokens[1:]

	for startIdx := 0; startIdx < len(burnTokens); startIdx += maxBurnBatchSize {
		endIdx := startIdx + maxBurnBatchSize
		if endIdx > len(burnTokens) {
			endIdx = len(burnTokens)
		}
		burnBatch := burnTokens[startIdx:endIdx]
		burnTxs, proofs, err := w.burnTokensForDC(ctx, acc, burnBatch, targetTokenBacklink, invariantPredicateArgs)
		if err != nil {
			return err
		}

		// if there's more to burn, update backlink to continue
		targetTokenBacklink, err = w.joinTokenForDC(ctx, acc, burnTxs, proofs, targetTokenBacklink, targetTokenID, invariantPredicateArgs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) joinTokenForDC(ctx context.Context, acc *account.AccountKey, burnTxs []*txsystem.Transaction, proofs []*block.BlockProof, targetTokenBacklink sdk.TxHash, targetTokenID sdk.UnitID, invariantPredicateArgs []*PredicateInput) (sdk.TxHash, error) {
	joinAttrs := &tokens.JoinFungibleTokenAttributes{
		BurnTransactions:             burnTxs,
		Proofs:                       proofs,
		Backlink:                     targetTokenBacklink,
		InvariantPredicateSignatures: nil,
	}

	sub, err := w.prepareTxSubmission(ctx, targetTokenID, joinAttrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
		signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, gtx)
		if err != nil {
			return err
		}
		joinAttrs.SetInvariantPredicateSignatures(signatures)
		return anypb.MarshalFrom(tx.TransactionAttributes, joinAttrs, proto.MarshalOptions{})
	})
	if err != nil {
		return nil, err
	}
	if err = sub.ToBatch(w.backend, acc.PubKey).SendTx(ctx, true); err != nil {
		return nil, err
	}
	return sub.TxHash, nil
}

func (w *Wallet) burnTokensForDC(ctx context.Context, acc *account.AccountKey, tokensToBurn []*twb.TokenUnit, nonce sdk.TxHash, invariantPredicateArgs []*PredicateInput) ([]*txsystem.Transaction, []*block.BlockProof, error) {
	burnBatch := txsubmitter.NewBatch(acc.PubKey, w.backend)

	for _, token := range tokensToBurn {
		attrs := newBurnTxAttrs(token, nonce)
		sub, err := w.prepareTxSubmission(ctx, sdk.UnitID(token.ID), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
			signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, gtx)
			if err != nil {
				return err
			}
			attrs.SetInvariantPredicateSignatures(signatures)
			return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to prepare burn tx: %w", err)
		}
		burnBatch.Add(sub)
	}

	if err := burnBatch.SendTx(ctx, true); err != nil {
		return nil, nil, fmt.Errorf("failed to send burn tx: %w", err)
	}

	burnTxs := make([]*txsystem.Transaction, 0, len(burnBatch.Submissions()))
	proofs := make([]*block.BlockProof, 0, len(burnBatch.Submissions()))
	for _, sub := range burnBatch.Submissions() {
		burnTxs = append(burnTxs, sub.Transaction)
		proofs = append(proofs, sub.Proof.Proof)
	}
	return burnTxs, proofs, nil
}

func (w *Wallet) getTokensForDC(ctx context.Context, key sdk.PubKey, allowedTokenTypes []twb.TokenTypeID) (map[string][]*twb.TokenUnit, error) {
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
		typeID := string(tok.TypeID)
		tokenz, found := tokensByTypes[typeID]
		if !found && len(allowedTokenTypes) > 0 {
			// if filter is set, skip tokens of other types
			continue
		}
		tokensByTypes[typeID] = append(tokenz, tok)
	}
	for k, v := range tokensByTypes {
		if len(v) < 2 { // not interested if tokens count is less than two
			delete(tokensByTypes, k)
		}
	}
	return tokensByTypes, nil
}
