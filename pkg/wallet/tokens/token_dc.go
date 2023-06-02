package tokens

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/fxamacker/cbor/v2"
)

const maxBurnBatchSize = 100

func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64, allowedTokenTypes []twb.TokenTypeID, invariantPredicateArgs []*PredicateInput) error {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return err
	}

	for _, key := range keys {
		tokensByTypes, err := w.getTokensForDC(ctx, key.PubKey, allowedTokenTypes)
		if err != nil {
			return err
		}
		for _, tokenz := range tokensByTypes {
			if err = w.collectDust(ctx, key.AccountKey, tokenz, invariantPredicateArgs); err != nil {
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
	targetToken := typedTokens[0]
	targetTokenID := sdk.UnitID(targetToken.ID)
	targetTokenBacklink := targetToken.TxHash
	totalAmountJoined := targetToken.Amount
	burnTokens := typedTokens[1:]

	for startIdx := 0; startIdx < len(burnTokens); startIdx += maxBurnBatchSize {
		endIdx := startIdx + maxBurnBatchSize
		if endIdx > len(burnTokens) {
			endIdx = len(burnTokens)
		}
		burnBatch := burnTokens[startIdx:endIdx]

		// check batch overflow before burning the tokens
		totalAmountToBeJoined := totalAmountJoined
		for _, token := range burnBatch {
			sum := totalAmountToBeJoined + token.Amount
			if sum < totalAmountToBeJoined {
				log.Error("uint64 overflow when joining tokens of type '%X', account key '0x%X'", token.TypeID, acc.PubKey)
				// just stop without returning error, so that we can continue with other token types
				return nil
			}
			totalAmountToBeJoined = sum
		}

		burnBatchAmount, proofs, err := w.burnTokensForDC(ctx, acc, burnBatch, targetTokenBacklink, invariantPredicateArgs)
		if err != nil {
			return err
		}

		// if there's more to burn, update backlink to continue
		targetTokenBacklink, err = w.joinTokenForDC(ctx, acc, proofs, targetTokenBacklink, targetTokenID, invariantPredicateArgs)
		if err != nil {
			return err
		}
		totalAmountJoined += burnBatchAmount
	}
	return nil
}

func (w *Wallet) joinTokenForDC(ctx context.Context, acc *account.AccountKey, burnProofs []*sdk.Proof, targetTokenBacklink sdk.TxHash, targetTokenID sdk.UnitID, invariantPredicateArgs []*PredicateInput) (sdk.TxHash, error) {
	burnTxs := make([]*types.TransactionRecord, len(burnProofs))
	burnTxProofs := make([]*types.TxProof, len(burnProofs))
	for i, proof := range burnProofs {
		burnTxs[i] = proof.TxRecord
		burnTxProofs[i] = proof.TxProof
	}

	joinAttrs := &tokens.JoinFungibleTokenAttributes{
		BurnTransactions:             burnTxs,
		Proofs:                       burnTxProofs,
		Backlink:                     targetTokenBacklink,
		InvariantPredicateSignatures: nil,
	}

	sub, err := w.prepareTxSubmission(ctx, tokens.PayloadTypeJoinFungibleToken, joinAttrs, targetTokenID, acc, w.GetRoundNumber, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, tx, joinAttrs)
		if err != nil {
			return err
		}
		joinAttrs.SetInvariantPredicateSignatures(signatures)
		tx.Payload.Attributes, err = cbor.Marshal(joinAttrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = sub.ToBatch(w.backend, acc.PubKey).SendTx(ctx, true); err != nil {
		return nil, err
	}
	return sub.TxHash, nil
}

func (w *Wallet) burnTokensForDC(ctx context.Context, acc *account.AccountKey, tokensToBurn []*twb.TokenUnit, nonce sdk.TxHash, invariantPredicateArgs []*PredicateInput) (uint64, []*sdk.Proof, error) {
	burnBatch := txsubmitter.NewBatch(acc.PubKey, w.backend)
	rnFetcher := &cachingRoundNumberFetcher{delegate: w.GetRoundNumber}
	burnBatchAmount := uint64(0)

	for _, token := range tokensToBurn {
		burnBatchAmount += token.Amount
		attrs := newBurnTxAttrs(token, nonce)
		sub, err := w.prepareTxSubmission(ctx, tokens.PayloadTypeBurnFungibleToken, attrs, sdk.UnitID(token.ID), acc, rnFetcher.getRoundNumber, func(tx *types.TransactionOrder) error {
			signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, tx, attrs)
			if err != nil {
				return err
			}
			attrs.SetInvariantPredicateSignatures(signatures)
			tx.Payload.Attributes, err = cbor.Marshal(attrs)
			return err
		})
		if err != nil {
			return 0, nil, fmt.Errorf("failed to prepare burn tx: %w", err)
		}
		burnBatch.Add(sub)
	}

	if err := burnBatch.SendTx(ctx, true); err != nil {
		return 0, nil, fmt.Errorf("failed to send burn tx: %w", err)
	}

	proofs := make([]*sdk.Proof, 0, len(burnBatch.Submissions()))
	for _, sub := range burnBatch.Submissions() {
		proofs = append(proofs, sub.Proof)
	}
	return burnBatchAmount, proofs, nil
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
