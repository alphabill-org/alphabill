package twb

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type txProcessor struct {
	store Storage
	txs   txsystem.TransactionSystem
}

func (t *txProcessor) readTx(tx *txsystem.Transaction, b *block.Block) error {
	gtx, err := t.txs.ConvertTx(tx)
	if err != nil {
		return err
	}
	id := util.Uint256ToBytes(gtx.UnitID())
	txHash := gtx.Hash(crypto.SHA256)
	log.Info(fmt.Sprintf("Converted tx: UnitId=%X, TxId=%X", id, txHash))

	proof, err := t.createProof(id, b, tx)
	if err != nil {
		return errors.Wrapf(err, "failed to create proof for tx with id=%X", txHash)
	}

	switch ctx := gtx.(type) {
	case tokens.CreateFungibleTokenType:
		log.Info("CreateFungibleTokenType tx")
		err = t.saveTokenType(&TokenUnitType{
			Kind:                     Fungible,
			ID:                       id,
			ParentTypeID:             ctx.ParentTypeID(),
			Symbol:                   ctx.Symbol(),
			DecimalPlaces:            ctx.DecimalPlaces(),
			SubTypeCreationPredicate: ctx.SubTypeCreationPredicate(),
			TokenCreationPredicate:   ctx.TokenCreationPredicate(),
			InvariantPredicate:       ctx.InvariantPredicate(),
			TxHash:                   txHash,
		}, proof)
		if err != nil {
			return err
		}
	case tokens.MintFungibleToken:
		log.Info("MintFungibleToken tx")
		tokenType, err := t.store.GetTokenType(ctx.TypeID())
		if err != nil {
			return errors.Wrapf(err, "mint fungible token tx: failed to get token type with id=%X, token id=%X", ctx.TypeID(), id)
		}

		newToken := &TokenUnit{
			ID:       id,
			Kind:     Fungible,
			TypeID:   ctx.TypeID(),
			Amount:   ctx.Value(),
			Symbol:   tokenType.Symbol,
			Decimals: tokenType.DecimalPlaces,
			TxHash:   txHash,
			Owner:    ctx.Bearer(),
		}

		if err = t.saveToken(newToken, proof); err != nil {
			return err
		}
	case tokens.TransferFungibleToken:
		log.Info("TransferFungibleToken tx")
		token, err := t.store.GetToken(id)
		if err != nil {
			return errors.Wrapf(err, "fungible transfer tx: failed to get token with id=%X", id)
		}
		token.TxHash = txHash
		token.Owner = ctx.NewBearer()
		if err = t.saveToken(token, proof); err != nil {
			return err
		}
	case tokens.SplitFungibleToken:
		log.Info("SplitFungibleToken tx")
		// check and update existing token
		token, err := t.store.GetToken(id)
		if err != nil {
			return errors.Wrapf(err, "split tx: failed to get token with id=%X", id)
		}
		if !bytes.Equal(token.TypeID, ctx.TypeID()) {
			return errors.Errorf("split tx: type id does not match (received '%X', expected '%X'), token id=%X", ctx.TypeID(), token.TypeID, token.ID)
		}
		remainingValue := token.Amount - ctx.TargetValue()
		if ctx.RemainingValue() != remainingValue {
			return errors.Errorf("split tx: invalid remaining amount (received '%v', expected '%v'), token id=%X", ctx.RemainingValue(), remainingValue, token.ID)
		}

		token.Amount = remainingValue
		token.TxHash = txHash
		if err = t.saveToken(token, proof); err != nil {
			return err
		}

		// save new token created by the split
		newId := txutil.SameShardIDBytes(ctx.UnitID(), ctx.HashForIDCalculation(crypto.SHA256))
		splitProof, err := t.createProof(newId, b, tx)
		if err != nil {
			return errors.Wrapf(err, "failed to create proof for split tx with id=%X for unit %X", txHash, newId)
		}
		newToken := &TokenUnit{
			ID:       newId,
			Symbol:   token.Symbol,
			TypeID:   token.TypeID,
			Kind:     token.Kind,
			Amount:   ctx.TargetValue(),
			Decimals: token.Decimals,
			TxHash:   txHash,
			Owner:    ctx.NewBearer(),
		}
		if err = t.saveToken(newToken, splitProof); err != nil {
			return err
		}
	case tokens.BurnFungibleToken:
		log.Info("Token tx: BurnFungibleToken")
		panic("not implemented") // TODO in 0.2.0
	case tokens.JoinFungibleToken:
		log.Info("Token tx: JoinFungibleToken")
		panic("not implemented") // TODO in 0.2.0
	case tokens.CreateNonFungibleTokenType:
		log.Info("Token tx: CreateNonFungibleTokenType")
		err := t.saveTokenType(&TokenUnitType{
			Kind:                     NonFungible,
			ID:                       id,
			ParentTypeID:             ctx.ParentTypeID(),
			Symbol:                   ctx.Symbol(),
			SubTypeCreationPredicate: ctx.SubTypeCreationPredicate(),
			TokenCreationPredicate:   ctx.TokenCreationPredicate(),
			InvariantPredicate:       ctx.InvariantPredicate(),
			NftDataUpdatePredicate:   ctx.DataUpdatePredicate(),
			TxHash:                   txHash,
		}, proof)
		if err != nil {
			return err
		}
	case tokens.MintNonFungibleToken:
		log.Info("Token tx: MintNonFungibleToken")
		tokenType, err := t.store.GetTokenType(ctx.NFTTypeID())
		if err != nil {
			return errors.Wrapf(err, "mint nft tx: failed to get token type with id=%X, token id=%X", ctx.NFTTypeID(), id)
		}

		newToken := &TokenUnit{
			ID:                     id,
			Kind:                   NonFungible,
			TypeID:                 ctx.NFTTypeID(),
			Symbol:                 tokenType.Symbol,
			NftURI:                 ctx.URI(),
			NftData:                ctx.Data(),
			NftDataUpdatePredicate: ctx.DataUpdatePredicate(),
			TxHash:                 txHash,
			Owner:                  ctx.Bearer(),
		}

		if err = t.saveToken(newToken, proof); err != nil {
			return err
		}
	case tokens.TransferNonFungibleToken:
		log.Info("Token tx: TransferNonFungibleToken")
		token, err := t.store.GetToken(id)
		if err != nil {
			return errors.Wrapf(err, "transfer nft tx: failed to get token with id=%X", id)
		}
		token.Owner = ctx.NewBearer()
		if err = t.saveToken(token, proof); err != nil {
			return err
		}
	case tokens.UpdateNonFungibleToken:
		log.Info("Token tx: UpdateNonFungibleToken")
		token, err := t.store.GetToken(id)
		if err != nil {
			return errors.Wrapf(err, "update nft tx: failed to get token with id=%X", id)
		}
		token.NftData = ctx.Data()
		token.TxHash = txHash
		if err = t.saveToken(token, proof); err != nil {
			return err
		}
	default:
		log.Warning(fmt.Sprintf("received unknown token transaction type, skipped processing: %s", ctx))
		return nil
	}
	return nil
}

func (t *txProcessor) saveTokenType(unit *TokenUnitType, proof *Proof) error {
	return t.store.SaveTokenType(unit, proof)
}

func (t *txProcessor) saveToken(unit *TokenUnit, proof *Proof) error {
	return t.store.SaveToken(unit, proof)
}

func (t *txProcessor) createProof(unitID []byte, b *block.Block, tx *txsystem.Transaction) (*Proof, error) {
	if b == nil {
		return nil, nil
	}
	gblock, err := b.ToGenericBlock(t.txs)
	if err != nil {
		return nil, err
	}
	proof, err := block.NewPrimaryProof(gblock, unitID, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	return &Proof{
		BlockNumber: b.BlockNumber,
		Tx:          tx,
		Proof:       proof,
	}, nil
}
