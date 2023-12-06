package tokens

import (
	"context"
	"crypto"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"

	sdk "github.com/alphabill-org/alphabill/wallet"
	"github.com/alphabill-org/alphabill/wallet/account"
	twb "github.com/alphabill-org/alphabill/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/wallet/txsubmitter"
)

const maxBurnBatchSize = 100

func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64, allowedTokenTypes []twb.TokenTypeID, invariantPredicateArgs []*PredicateInput) ([]*SubmissionResult, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}
	results := make([]*SubmissionResult, 0, len(keys))

	for _, key := range keys {
		tokensByTypes, err := w.getTokensForDC(ctx, key.PubKey, allowedTokenTypes)
		if err != nil {
			return nil, err
		}
		for _, tokenz := range tokensByTypes {
			result, err := w.collectDust(ctx, key, tokenz, invariantPredicateArgs)
			if err != nil {
				return results, err
			}
			if result != nil {
				results = append(results, result)
			}
		}
	}
	return results, nil
}

func (w *Wallet) collectDust(ctx context.Context, acc *accountKey, typedTokens []*twb.TokenUnit, invariantPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	batchCount := ((len(typedTokens) - 1) / maxBurnBatchSize) + 1
	txCount := len(typedTokens) + batchCount*2 // +lock fee and join fee for every batch
	if err := w.ensureFeeCredit(ctx, acc.AccountKey, txCount); err != nil {
		return nil, err
	}
	// first token to be joined into
	targetToken := typedTokens[0]
	targetTokenID := targetToken.ID
	targetTokenBacklink := targetToken.TxHash
	totalAmountJoined := targetToken.Amount
	burnTokens := typedTokens[1:]
	totalFees := uint64(0)

	for startIdx := 0; startIdx < len(burnTokens); startIdx += maxBurnBatchSize {
		endIdx := startIdx + maxBurnBatchSize
		if endIdx > len(burnTokens) {
			endIdx = len(burnTokens)
		}
		burnBatch := burnTokens[startIdx:endIdx]

		// check batch overflow before burning the tokens
		totalAmountToBeJoined := totalAmountJoined
		var err error
		for _, token := range burnBatch {
			totalAmountToBeJoined, _, err = util.AddUint64(totalAmountToBeJoined, token.Amount)
			if err != nil {
				w.log.WarnContext(ctx, fmt.Sprintf("unable to join tokens of type '%X', account key '0x%X': %v", token.TypeID, acc.PubKey, err))
				// just stop without returning error, so that we can continue with other token types
				if totalFees > 0 {
					return &SubmissionResult{FeeSum: totalFees, AccountNumber: acc.idx + 1}, nil
				}
				return nil, nil
			}
		}

		var lockFee uint64
		targetTokenBacklink, lockFee, err = w.lockTokenForDC(ctx, acc.AccountKey, targetTokenID, targetTokenBacklink, invariantPredicateArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to lock target token: %w", err)
		}

		burnBatchAmount, burnFee, proofs, err := w.burnTokensForDC(ctx, acc.AccountKey, burnBatch, targetTokenBacklink, targetTokenID, invariantPredicateArgs)
		if err != nil {
			return nil, err
		}

		// if there's more to burn, update backlink to continue
		var joinFee uint64
		targetTokenBacklink, joinFee, err = w.joinTokenForDC(ctx, acc.AccountKey, proofs, targetTokenBacklink, targetTokenID, invariantPredicateArgs)
		if err != nil {
			return nil, err
		}

		totalAmountJoined += burnBatchAmount
		totalFees += lockFee + burnFee + joinFee
	}
	return &SubmissionResult{FeeSum: totalFees, AccountNumber: acc.idx + 1}, nil
}

func (w *Wallet) joinTokenForDC(ctx context.Context, acc *account.AccountKey, burnProofs []*sdk.Proof, targetTokenBacklink sdk.TxHash, targetTokenID types.UnitID, invariantPredicateArgs []*PredicateInput) (sdk.TxHash, uint64, error) {
	// explicitly sort proofs by unit ids in increasing order, even though backend already returns tokens ordered by id
	sort.Slice(burnProofs, func(i, j int) bool {
		a := burnProofs[i].TxRecord.TransactionOrder.UnitID()
		b := burnProofs[j].TxRecord.TransactionOrder.UnitID()
		return a.Compare(b) < 0
	})
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
		return nil, 0, err
	}
	if err = sub.ToBatch(w.backend, acc.PubKey, w.log).SendTx(ctx, true); err != nil {
		return nil, 0, err
	}
	return sub.Proof.TxRecord.TransactionOrder.Hash(crypto.SHA256), sub.Proof.TxRecord.ServerMetadata.ActualFee, nil
}

func (w *Wallet) burnTokensForDC(ctx context.Context, acc *account.AccountKey, tokensToBurn []*twb.TokenUnit, targetTokenBacklink sdk.TxHash, targetTokenID types.UnitID, invariantPredicateArgs []*PredicateInput) (uint64, uint64, []*sdk.Proof, error) {
	burnBatch := txsubmitter.NewBatch(acc.PubKey, w.backend, w.log)
	rnFetcher := &cachingRoundNumberFetcher{delegate: w.GetRoundNumber}
	burnBatchAmount := uint64(0)

	for _, token := range tokensToBurn {
		burnBatchAmount += token.Amount
		attrs := newBurnTxAttrs(token, targetTokenBacklink, targetTokenID)
		sub, err := w.prepareTxSubmission(ctx, tokens.PayloadTypeBurnFungibleToken, attrs, token.ID, acc, rnFetcher.getRoundNumber, func(tx *types.TransactionOrder) error {
			signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, tx, attrs)
			if err != nil {
				return err
			}
			attrs.SetInvariantPredicateSignatures(signatures)
			tx.Payload.Attributes, err = cbor.Marshal(attrs)
			return err
		})
		if err != nil {
			return 0, 0, nil, fmt.Errorf("failed to prepare burn tx: %w", err)
		}
		burnBatch.Add(sub)
	}

	if err := burnBatch.SendTx(ctx, true); err != nil {
		return 0, 0, nil, fmt.Errorf("failed to send burn tx: %w", err)
	}

	proofs := make([]*sdk.Proof, 0, len(burnBatch.Submissions()))
	feeSum := uint64(0)
	for _, sub := range burnBatch.Submissions() {
		proofs = append(proofs, sub.Proof)
		feeSum += sub.Proof.TxRecord.ServerMetadata.ActualFee
	}
	return burnBatchAmount, feeSum, proofs, nil
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

func (w *Wallet) lockTokenForDC(ctx context.Context, acc *account.AccountKey, targetTokenID types.UnitID, targetTokenBacklink sdk.TxHash, invariantPredicateArgs []*PredicateInput) (sdk.TxHash, uint64, error) {
	attr := &tokens.LockTokenAttributes{
		LockStatus: sdk.LockReasonCollectDust,
		Backlink:   targetTokenBacklink,
	}

	sub, err := w.prepareTxSubmission(ctx, tokens.PayloadTypeLockToken, attr, targetTokenID, acc, w.GetRoundNumber, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, tx, attr)
		if err != nil {
			return err
		}
		attr.InvariantPredicateSignatures = signatures
		tx.Payload.Attributes, err = cbor.Marshal(attr)
		return err
	})
	if err != nil {
		return nil, 0, err
	}

	if err = sub.ToBatch(w.backend, acc.PubKey, w.log).SendTx(ctx, true); err != nil {
		return nil, 0, err
	}
	return sub.Proof.TxRecord.TransactionOrder.Hash(crypto.SHA256), sub.Proof.TxRecord.ServerMetadata.ActualFee, nil
}
