package twb

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	wtokens "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
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

	switch ctx := gtx.(type) {
	case tokens.CreateFungibleTokenType:
		log.Info("CreateFungibleTokenType tx")
		err = w.addTokenTypeWithProof(&TokenUnitType{
			ID:            id,
			Kind:          FungibleTokenType,
			Symbol:        ctx.Symbol(),
			ParentTypeID:  ctx.ParentTypeID(),
			DecimalPlaces: ctx.DecimalPlaces(),
		}, b, tx, txc)
		if err != nil {
			return err
		}
	case tokens.MintFungibleToken:
		log.Info("MintFungibleToken tx")
		if checkOwner(accNr, key, ctx.Bearer()) {
			tType, err := txc.GetTokenType(ctx.TypeID())
			if err != nil {
				return err
			}
			if tType == nil {
				return errors.Errorf("mint fungible token tx: token type with id=%X not found, token id=%X", ctx.TypeID(), id)
			}
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				Kind:     FungibleToken,
				TypeID:   ctx.TypeID(),
				Amount:   ctx.Value(),
				Backlink: txHash,
				Symbol:   tType.Symbol,
			}, b, tx, txc)
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.TransferFungibleToken:
		log.Info("TransferFungibleToken tx")
		if checkOwner(accNr, key, ctx.NewBearer()) {
			tokenInfo, err := txc.GetTokenType(ctx.TypeID())
			if err != nil {
				return err
			}
			if tokenInfo == nil {
				return errors.Errorf("fungible transfer tx: token type with id=%X not found, token id=%X", ctx.TypeID(), id)
			}
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				TypeID:   ctx.TypeID(),
				Kind:     FungibleToken,
				Amount:   ctx.Value(),
				Symbol:   tokenInfo.Symbol,
				Backlink: txHash,
			}, b, tx, txc)
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.SplitFungibleToken:
		log.Info("SplitFungibleToken tx")
		tok, err := txc.GetToken(accNr, id)
		if err != nil {
			return err
		}
		var tokenInfo TokenTypeInfo
		if tok != nil {
			tokenInfo = tok
			log.Info("SplitFungibleToken updating existing unit")
			if !bytes.Equal(tok.TypeID, ctx.TypeID()) {
				return errors.Errorf("split tx: type id does not match (received '%X', expected '%X'), token id=%X", ctx.TypeID(), tok.TypeID, tok.ID)
			}
			remainingValue := tok.Amount - ctx.TargetValue()
			if ctx.RemainingValue() != remainingValue {
				return errors.Errorf("split tx: invalid remaining amount (received '%v', expected '%v'), token id=%X", ctx.RemainingValue(), remainingValue, tok.ID)
			}
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				Symbol:   tok.Symbol,
				TypeID:   tok.TypeID,
				Kind:     tok.Kind,
				Amount:   tok.Amount - ctx.TargetValue(),
				Backlink: txHash,
			}, b, tx, txc)
			if err != nil {
				return err
			}
		} else {
			tokenInfo, err = txc.GetTokenType(ctx.TypeID())
			if err != nil {
				return err
			}
			if tokenInfo == nil {
				return errors.Errorf("split tx: token type with id=%X not found, token id=%X", ctx.TypeID(), id)
			}
		}

		if checkOwner(accNr, key, ctx.NewBearer()) {
			newId := txutil.SameShardIDBytes(ctx.UnitID(), ctx.HashForIDCalculation(crypto.SHA256))
			log.Info(fmt.Sprintf("SplitFungibleToken: adding new unit from split, new UnitId=%X", newId))
			err := w.addTokenWithProof(accNr, &TokenUnit{
				ID:       newId,
				Symbol:   tokenInfo.GetSymbol(),
				TypeID:   tokenInfo.GetTypeId(),
				Kind:     FungibleToken,
				Amount:   ctx.TargetValue(),
				Backlink: txHash,
			}, b, tx, txc)
			if err != nil {
				return err
			}
		}
	case tokens.BurnFungibleToken:
		log.Info("Token tx: BurnFungibleToken")
		panic("not implemented") // TODO
	case tokens.JoinFungibleToken:
		log.Info("Token tx: JoinFungibleToken")
		panic("not implemented") // TODO
	case tokens.CreateNonFungibleTokenType:
		log.Info("Token tx: CreateNonFungibleTokenType")
		err := w.addTokenTypeWithProof(&TokenUnitType{
			ID:           id,
			Kind:         NonFungibleTokenType,
			Symbol:       ctx.Symbol(),
			ParentTypeID: ctx.ParentTypeID(),
		}, b, tx, txc)
		if err != nil {
			return err
		}
	case tokens.MintNonFungibleToken:
		log.Info("Token tx: MintNonFungibleToken")
		if checkOwner(accNr, key, ctx.Bearer()) {
			tType, err := txc.GetTokenType(ctx.NFTTypeID())
			if err != nil {
				return err
			}
			if tType == nil {
				return errors.Errorf("mint nft tx: token type with id=%X not found, token id=%X", ctx.NFTTypeID(), id)
			}
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				Kind:     NonFungibleToken,
				TypeID:   ctx.NFTTypeID(),
				URI:      ctx.URI(),
				Backlink: txHash,
				Symbol:   tType.Symbol,
			}, b, tx, txc)
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.TransferNonFungibleToken:
		log.Info("Token tx: TransferNonFungibleToken")
		if checkOwner(accNr, key, ctx.NewBearer()) {
			tType, err := txc.GetTokenType(ctx.NFTTypeID())
			if err != nil {
				return err
			}
			if tType == nil {
				return errors.Errorf("transfer nft tx: token type with id=%X not found, token id=%X", ctx.NFTTypeID(), id)
			}
			err = w.addTokenWithProof(accNr, &TokenUnit{
				ID:       id,
				TypeID:   ctx.NFTTypeID(),
				Kind:     NonFungibleToken,
				Backlink: txHash,
				Symbol:   tType.Symbol,
			}, b, tx, txc)
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.UpdateNonFungibleToken:
		log.Info("Token tx: UpdateNonFungibleToken")
		tok, err := txc.GetToken(accNr, id)
		if err != nil {
			return err
		}
		if tok != nil {
			tok.Backlink = txHash
			if err = w.addTokenWithProof(accNr, tok, b, tx, txc); err != nil {
				return err
			}
		}
	default:
		log.Warning(fmt.Sprintf("received unknown token transaction type, skipped processing: %s", ctx))
		return nil
	}
	return nil
}

func (t *txProcessor) createProof(b *block.Block, tx *txsystem.Transaction) (*wtokens.Proof, error) {
	if b == nil {
		return nil, nil
	}
	gblock, err := b.ToGenericBlock(t.txs)
	if err != nil {
		return nil, fmt.Errorf("failed to convert block to generic block: %w", err)
	}
	proof, err := block.NewPrimaryProof(gblock, tx.UnitId, crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary proof for the block: %w", err)
	}
	return &wtokens.Proof{
		BlockNumber: b.BlockNumber,
		Tx:          tx,
		Proof:       proof,
	}, nil
}

func (t *txProcessor) saveNonFungibleTokenTx(tx txsystem.GenericTransaction, proof *wtokens.Proof) error {
	type nfTokenTx interface {
		NFTTypeID() []byte
	}
	ttx := tx.(nfTokenTx)

	tType, err := t.store.GetTokenType(ttx.NFTTypeID())
	if err != nil {
		return err
	}

	d := &wtokens.TokenUnit{
		ID:       util.Uint256ToBytes(tx.UnitID()),
		Kind:     wtokens.NonFungibleToken,
		TypeID:   ttx.NFTTypeID(),
		Backlink: tx.Hash(crypto.SHA256),
		Symbol:   tType.Symbol,
		Proof:    proof,
	}
	if u, ok := tx.(interface{ URI() string }); ok {
		d.URI = u.URI()
	}
	if err := t.store.SaveTokenUnit(d); err != nil {
		return fmt.Errorf("failed to save %s (%x): %w", d.Kind, d.ID, err)
	}
	return nil
}

func (t *txProcessor) saveFungibleTokenTx(tx txsystem.GenericTransaction, proof *wtokens.Proof) error {
	type fungibleTokenTx interface {
		TypeID() []byte
		Value() uint64
	}
	ttx := tx.(fungibleTokenTx)

	tType, err := t.store.GetTokenType(ttx.TypeID())
	if err != nil {
		return err
	}

	d := &wtokens.TokenUnit{
		ID:       util.Uint256ToBytes(tx.UnitID()),
		Kind:     wtokens.FungibleToken,
		TypeID:   ttx.TypeID(),
		Amount:   ttx.Value(),
		Backlink: tx.Hash(crypto.SHA256),
		Symbol:   tType.Symbol,
		Proof:    proof,
	}
	if err := t.store.SaveTokenUnit(d); err != nil {
		return fmt.Errorf("failed to save %s (%x): %w", d.Kind, d.ID, err)
	}
	return nil
}

func (t *txProcessor) saveTokenTypeTx(tx txsystem.GenericTransaction) error {
	type tokenTypeTx interface {
		Symbol() string
		ParentTypeID() []byte
	}
	ttx := tx.(tokenTypeTx)
	d := &wtokens.TokenUnitType{
		ID:           util.Uint256ToBytes(tx.UnitID()),
		Kind:         wtokens.NonFungibleTokenType,
		Symbol:       ttx.Symbol(),
		ParentTypeID: ttx.ParentTypeID(),
	}
	if err := t.store.SaveTokenType(d); err != nil {
		return fmt.Errorf("failed to save %s (%x): %w", d.Kind, d.ID, err)
	}
	return nil
}
