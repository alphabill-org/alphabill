package tokens

import (
	"context"
	"crypto"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	"github.com/fxamacker/cbor/v2"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
)

type (
	txPreprocessor     func(tx *types.TransactionOrder) error
	roundNumberFetcher func(ctx context.Context) (uint64, error)

	cachingRoundNumberFetcher struct {
		roundNumber uint64
		delegate    roundNumberFetcher
	}
)

func (f *cachingRoundNumberFetcher) getRoundNumber(ctx context.Context) (uint64, error) {
	if f.roundNumber == 0 {
		var err error
		f.roundNumber, err = f.delegate(ctx)
		if err != nil {
			return 0, err
		}
	}
	return f.roundNumber, nil
}

func (w *Wallet) newType(ctx context.Context, accNr uint64, payloadType string, attrs AttrWithSubTypeCreationInputs, typeId backend.TokenTypeID, subtypePredicateArgs []*PredicateInput) (*txsubmitter.TxSubmission, error) {
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	acc, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}
	err = w.ensureFeeCredit(ctx, acc, 1)
	if err != nil {
		return nil, err
	}
	sub, err := w.prepareTxSubmission(ctx, payloadType, attrs, typeId, acc, w.GetRoundNumber, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, subtypePredicateArgs, tx, attrs)
		if err != nil {
			return err
		}
		attrs.SetSubTypeCreationPredicateSignatures(signatures)
		tx.Payload.Attributes, err = cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.backend, acc.PubKey, w.log).SendTx(ctx, w.confirmTx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func preparePredicateSignatures(am account.Manager, args []*PredicateInput, tx *types.TransactionOrder, attrs types.SigBytesProvider) ([][]byte, error) {
	signatures := make([][]byte, 0, len(args))
	for _, input := range args {
		if input.AccountNumber > 0 {
			ac, err := am.GetAccountKey(input.AccountNumber - 1)
			if err != nil {
				return nil, err
			}
			sig, err := signTx(tx, attrs, ac)
			if err != nil {
				return nil, err
			}
			signatures = append(signatures, sig)
		} else {
			signatures = append(signatures, input.Argument)
		}
	}
	return signatures, nil
}

func (w *Wallet) newToken(ctx context.Context, accNr uint64, payloadType string, attrs MintAttr, tokenId backend.TokenID, mintPredicateArgs []*PredicateInput) (*txsubmitter.TxSubmission, error) {
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	key, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, err
	}
	err = w.ensureFeeCredit(ctx, key, 1)
	if err != nil {
		return nil, err
	}
	sub, err := w.prepareTxSubmission(ctx, payloadType, attrs, tokenId, key, w.GetRoundNumber, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, mintPredicateArgs, tx, attrs)
		if err != nil {
			return err
		}
		attrs.SetTokenCreationPredicateSignatures(signatures)
		tx.Payload.Attributes, err = cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.backend, key.PubKey, w.log).SendTx(ctx, w.confirmTx)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (w *Wallet) prepareTxSubmission(ctx context.Context, payloadType string, attrs types.SigBytesProvider, unitId types.UnitID, ac *account.AccountKey, rn roundNumberFetcher, txps txPreprocessor) (*txsubmitter.TxSubmission, error) {
	w.log.InfoContext(ctx, fmt.Sprintf("Preparing to send token tx, UnitID=%s", unitId))

	roundNumber, err := rn(ctx)
	if err != nil {
		return nil, err
	}
	tx := createTx(w.systemID, payloadType, unitId, roundNumber+txTimeoutRoundCount, ttxs.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256))
	if txps != nil {
		// set fields before tx is signed
		err = txps(tx)
		if err != nil {
			return nil, err
		}
	}
	sig, err := signTx(tx, attrs, ac)
	if err != nil {
		return nil, err
	}
	tx.OwnerProof = sig

	// TODO: AB-1016 remove when fixed
	sig, err = makeTxFeeProof(tx, ac)
	if err != nil {
		return nil, fmt.Errorf("failed to make tx fee proof: %w", err)
	}
	tx.FeeProof = sig

	// convert again for hashing as the tx might have been modified
	txSub := &txsubmitter.TxSubmission{
		UnitID:      unitId,
		Transaction: tx,
		TxHash:      tx.Hash(crypto.SHA256),
	}
	return txSub, nil
}

func signTx(tx *types.TransactionOrder, attrs types.SigBytesProvider, ac *account.AccountKey) (wallet.Predicate, error) {
	if ac == nil {
		return nil, nil
	}
	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return nil, err
	}
	bytes, err := tx.Payload.BytesWithAttributeSigBytes(attrs) // TODO: AB-1016
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(bytes)
	if err != nil {
		return nil, err
	}
	return templates.NewP2pkh256SignatureBytes(sig, ac.PubKey), nil
}

func makeTxFeeProof(tx *types.TransactionOrder, ac *account.AccountKey) (wallet.Predicate, error) {
	if ac == nil {
		return nil, nil
	}
	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return nil, err
	}
	bytes, err := tx.Payload.Bytes()
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(bytes)
	if err != nil {
		return nil, err
	}
	return templates.NewP2pkh256SignatureBytes(sig, ac.PubKey), nil
}

func newFungibleTransferTxAttrs(token *backend.TokenUnit, receiverPubKey []byte) *ttxs.TransferFungibleTokenAttributes {
	return &ttxs.TransferFungibleTokenAttributes{
		TypeID:                       token.TypeID,
		NewBearer:                    BearerPredicateFromPubKey(receiverPubKey),
		Value:                        token.Amount,
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

func newNonFungibleTransferTxAttrs(token *backend.TokenUnit, receiverPubKey []byte) *ttxs.TransferNonFungibleTokenAttributes {
	return &ttxs.TransferNonFungibleTokenAttributes{
		NFTTypeID:                    token.TypeID,
		NewBearer:                    BearerPredicateFromPubKey(receiverPubKey),
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

func bearerPredicateFromHash(receiverPubKeyHash []byte) wallet.Predicate {
	var bytes []byte
	if receiverPubKeyHash != nil {
		bytes = templates.NewP2pkh256BytesFromKeyHash(receiverPubKeyHash)
	} else {
		bytes = templates.AlwaysTrueBytes()
	}
	return bytes
}

func BearerPredicateFromPubKey(receiverPubKey wallet.PubKey) wallet.Predicate {
	var h []byte
	if receiverPubKey != nil {
		h = hash.Sum256(receiverPubKey)
	}
	return bearerPredicateFromHash(h)
}

func newSplitTxAttrs(token *backend.TokenUnit, amount uint64, receiverPubKey []byte) *ttxs.SplitFungibleTokenAttributes {
	return &ttxs.SplitFungibleTokenAttributes{
		TypeID:                       token.TypeID,
		NewBearer:                    BearerPredicateFromPubKey(receiverPubKey),
		TargetValue:                  amount,
		RemainingValue:               token.Amount - amount,
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: [][]byte{nil},
	}
}

func newBurnTxAttrs(token *backend.TokenUnit, targetTokenBacklink wallet.TxHash, targetTokenID types.UnitID) *ttxs.BurnFungibleTokenAttributes {
	return &ttxs.BurnFungibleTokenAttributes{
		TypeID:                       token.TypeID,
		Value:                        token.Amount,
		TargetTokenID:                targetTokenID,
		TargetTokenBacklink:          targetTokenBacklink,
		Backlink:                     token.TxHash,
		InvariantPredicateSignatures: nil,
	}
}

// assumes there's sufficient balance for the given amount, sends transactions immediately
func (w *Wallet) doSendMultiple(ctx context.Context, amount uint64, tokens []*backend.TokenUnit, acc *account.AccountKey, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	var accumulatedSum uint64
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Amount > tokens[j].Amount
	})

	batch := txsubmitter.NewBatch(acc.PubKey, w.backend, w.log)
	rnFetcher := &cachingRoundNumberFetcher{delegate: w.GetRoundNumber}

	for _, t := range tokens {
		remainingAmount := amount - accumulatedSum
		sub, err := w.prepareSplitOrTransferTx(ctx, acc, remainingAmount, t, receiverPubKey, invariantPredicateArgs, rnFetcher.getRoundNumber)
		if err != nil {
			return nil, err
		}
		batch.Add(sub)
		accumulatedSum += t.Amount
		if accumulatedSum >= amount {
			break
		}
	}
	err := batch.SendTx(ctx, w.confirmTx)
	feeSum := uint64(0)
	for _, sub := range batch.Submissions() {
		if sub.Confirmed() {
			feeSum += sub.Proof.TxRecord.ServerMetadata.ActualFee
		}
	}
	return &SubmissionResult{FeeSum: feeSum}, err
}

func (w *Wallet) prepareSplitOrTransferTx(ctx context.Context, acc *account.AccountKey, amount uint64, token *backend.TokenUnit, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput, rn roundNumberFetcher) (*txsubmitter.TxSubmission, error) {
	var attrs AttrWithInvariantPredicateInputs
	var payloadType string
	if amount >= token.Amount {
		attrs = newFungibleTransferTxAttrs(token, receiverPubKey)
		payloadType = ttxs.PayloadTypeTransferFungibleToken
	} else {
		attrs = newSplitTxAttrs(token, amount, receiverPubKey)
		payloadType = ttxs.PayloadTypeSplitFungibleToken
	}
	sub, err := w.prepareTxSubmission(ctx, payloadType, attrs, token.ID, acc, rn, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, invariantPredicateArgs, tx, attrs)
		if err != nil {
			return err
		}
		attrs.SetInvariantPredicateSignatures(signatures)
		tx.Payload.Attributes, err = cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func createTx(systemID []byte, payloadType string, unitId []byte, timeout uint64, fcrID []byte) *types.TransactionOrder {
	return &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID: systemID,
			Type:     payloadType,
			UnitID:   unitId,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: tx_builder.MaxFee,
				FeeCreditRecordID: fcrID,
			},
		},
		// OwnerProof is added after whole transaction is built
	}
}
