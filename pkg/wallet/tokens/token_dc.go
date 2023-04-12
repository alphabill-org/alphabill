package tokens

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

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
	// TODO: rewrite with goroutines?
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
	// first token to be joined into
	targetTokenID := twb.UnitID(typedTokens[0].ID)
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

func (w *Wallet) joinTokenForDC(ctx context.Context, acc *account.AccountKey, burnTxs []*txsystem.Transaction, proofs []*block.BlockProof, targetTokenBacklink twb.TxHash, targetTokenID twb.UnitID, invariantPredicateArgs []*PredicateInput) (twb.TxHash, error) {
	joinAttrs := &tokens.JoinFungibleTokenAttributes{
		BurnTransactions:             burnTxs,
		Proofs:                       proofs,
		Backlink:                     targetTokenBacklink,
		InvariantPredicateSignatures: nil,
	}

	sub, err := w.prepareTx(ctx, targetTokenID, joinAttrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
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
	if err = sub.toBatch(w.backend, acc.PubKey).sendTx(ctx, true); err != nil {
		return nil, err
	}
	return sub.txHash, nil
}

func (w *Wallet) burnTokensForDC(ctx context.Context, acc *account.AccountKey, tokensToBurn []*twb.TokenUnit, nonce twb.TxHash, invariantPredicateArgs []*PredicateInput) (burnTxs []*txsystem.Transaction, proofs []*block.BlockProof, err error) {
	burnBatch := &txSubmissionBatch{
		sender:  acc.PubKey,
		backend: w.backend,
	}
	for _, token := range tokensToBurn {
		attrs := newBurnTxAttrs(token, nonce)
		var sub *txSubmission
		sub, err = w.prepareTx(ctx, twb.UnitID(token.ID), attrs, acc, func(tx *txsystem.Transaction, gtx txsystem.GenericTransaction) error {
			signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, gtx)
			if err != nil {
				return err
			}
			attrs.SetInvariantPredicateSignatures(signatures)
			return anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
		})
		if err != nil {
			return
		}
		burnBatch.add(sub)
	}
	if err = burnBatch.sendTx(ctx, true); err != nil {
		return
	}
	burnTxs = make([]*txsystem.Transaction, 0, len(burnBatch.submissions))
	proofs = make([]*block.BlockProof, 0, len(burnBatch.submissions))
	for _, sub := range burnBatch.submissions {
		burnTxs = append(burnTxs, sub.tx)
		proofs = append(proofs, sub.txProof)
	}
	return
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
