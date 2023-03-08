package twb

import (
	"bytes"
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
)

type blockProcessor struct {
	store  Storage
	txs    txsystem.TransactionSystem
	logErr func(a ...any)
}

func (p *blockProcessor) ProcessBlock(ctx context.Context, b *block.Block) error {
	lastBlockNumber, err := p.store.GetBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to read current block number: %w", err)
	}
	// block numbers must not be sequential (gaps might appear as empty block are not stored
	// and sent) but must be in ascending order
	if lastBlockNumber >= b.UnicityCertificate.InputRecord.RoundNumber {
		return fmt.Errorf("invalid block order: last processed block is %d, received block %d as next to process", lastBlockNumber, b.UnicityCertificate.InputRecord.RoundNumber)
	}

	for _, tx := range b.Transactions {
		if err := p.processTx(tx, b); err != nil {
			return fmt.Errorf("failed to process tx: %w", err)
		}
	}

	return p.store.SetBlockNumber(b.UnicityCertificate.InputRecord.RoundNumber)
}

func (p *blockProcessor) processTx(inTx *txsystem.Transaction, b *block.Block) error {
	gtx, err := p.txs.ConvertTx(inTx)
	if err != nil {
		return err
	}

	id := util.Uint256ToBytes(gtx.UnitID())
	txHash := gtx.Hash(crypto.SHA256)
	proof, err := p.createProof(id, b, inTx)
	if err != nil {
		return fmt.Errorf("failed to create proof for tx with id=%X: %w", txHash, err)
	}

	switch tx := gtx.(type) {
	case tokens.CreateFungibleTokenType:
		return p.saveTokenType(&TokenUnitType{
			Kind:                     Fungible,
			ID:                       id,
			ParentTypeID:             tx.ParentTypeID(),
			Symbol:                   tx.Symbol(),
			DecimalPlaces:            tx.DecimalPlaces(),
			SubTypeCreationPredicate: tx.SubTypeCreationPredicate(),
			TokenCreationPredicate:   tx.TokenCreationPredicate(),
			InvariantPredicate:       tx.InvariantPredicate(),
			TxHash:                   txHash,
		}, proof)
	case tokens.MintFungibleToken:
		tokenType, err := p.store.GetTokenType(tx.TypeID())
		if err != nil {
			return fmt.Errorf("mint fungible token tx: failed to get token type with id=%X, token id=%X: %w", tx.TypeID(), id, err)
		}
		return p.saveToken(
			&TokenUnit{
				ID:       id,
				TypeID:   tx.TypeID(),
				Amount:   tx.Value(),
				Kind:     tokenType.Kind,
				Symbol:   tokenType.Symbol,
				Decimals: tokenType.DecimalPlaces,
				TxHash:   txHash,
				Owner:    tx.Bearer(),
			},
			proof)
	case tokens.TransferFungibleToken:
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("fungible transfer tx: failed to get token with id=%X: %w", id, err)
		}
		token.TxHash = txHash
		token.Owner = tx.NewBearer()
		return p.saveToken(token, proof)
	case tokens.SplitFungibleToken:
		// check and update existing token
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("split tx: failed to get token with id=%X: %w", id, err)
		}
		if !bytes.Equal(token.TypeID, tx.TypeID()) {
			return fmt.Errorf("split tx: type id does not match (received '%X', expected '%X'), token id=%X", tx.TypeID(), token.TypeID, token.ID)
		}
		remainingValue := token.Amount - tx.TargetValue()
		if tx.RemainingValue() != remainingValue {
			return fmt.Errorf("split tx: invalid remaining amount (received '%v', expected '%v'), token id=%X", tx.RemainingValue(), remainingValue, token.ID)
		}

		token.Amount = remainingValue
		token.TxHash = txHash
		if err = p.saveToken(token, proof); err != nil {
			return err
		}

		// save new token created by the split
		newId := txutil.SameShardIDBytes(tx.UnitID(), tx.HashForIDCalculation(crypto.SHA256))
		splitProof, err := p.createProof(newId, b, inTx)
		if err != nil {
			return fmt.Errorf("failed to create proof for split tx with id=%X for unit %X: %w", txHash, newId, err)
		}
		newToken := &TokenUnit{
			ID:       newId,
			Symbol:   token.Symbol,
			TypeID:   token.TypeID,
			Kind:     token.Kind,
			Amount:   tx.TargetValue(),
			Decimals: token.Decimals,
			TxHash:   txHash,
			Owner:    tx.NewBearer(),
		}
		return p.saveToken(newToken, splitProof)
	//case tokens.BurnFungibleToken: // TODO in 0.2.0
	//case tokens.JoinFungibleToken: // TODO in 0.2.0
	case tokens.CreateNonFungibleTokenType:
		return p.saveTokenType(&TokenUnitType{
			Kind:                     NonFungible,
			ID:                       id,
			ParentTypeID:             tx.ParentTypeID(),
			Symbol:                   tx.Symbol(),
			SubTypeCreationPredicate: tx.SubTypeCreationPredicate(),
			TokenCreationPredicate:   tx.TokenCreationPredicate(),
			InvariantPredicate:       tx.InvariantPredicate(),
			NftDataUpdatePredicate:   tx.DataUpdatePredicate(),
			TxHash:                   txHash,
		}, proof)
	case tokens.MintNonFungibleToken:
		tokenType, err := p.store.GetTokenType(tx.NFTTypeID())
		if err != nil {
			return fmt.Errorf("mint nft tx: failed to get token type with id=%X, token id=%X: %w", tx.NFTTypeID(), id, err)
		}

		newToken := &TokenUnit{
			ID:                     id,
			Kind:                   tokenType.Kind,
			TypeID:                 tx.NFTTypeID(),
			Symbol:                 tokenType.Symbol,
			NftURI:                 tx.URI(),
			NftData:                tx.Data(),
			NftDataUpdatePredicate: tx.DataUpdatePredicate(),
			TxHash:                 txHash,
			Owner:                  tx.Bearer(),
		}
		return p.saveToken(newToken, proof)
	case tokens.TransferNonFungibleToken:
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("transfer nft tx: failed to get token with id=%X: %w", id, err)
		}
		token.Owner = tx.NewBearer()
		return p.saveToken(token, proof)
	case tokens.UpdateNonFungibleToken:
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("update nft tx: failed to get token with id=%X: %w", id, err)
		}
		token.NftData = tx.Data()
		token.TxHash = txHash
		return p.saveToken(token, proof)
	default:
		if p.logErr != nil {
			p.logErr("received unknown token transaction type, skipped processing:", tx)
		}
		return nil
	}
}

func (p *blockProcessor) saveTokenType(unit *TokenUnitType, proof *Proof) error {
	if err := p.store.SaveTokenType(unit, proof); err != nil {
		return fmt.Errorf("failed to store token type: %w", err)
	}
	return nil
}

func (p *blockProcessor) saveToken(unit *TokenUnit, proof *Proof) error {
	if err := p.store.SaveToken(unit, proof); err != nil {
		return fmt.Errorf("failed to store token: %w", err)
	}
	return nil
}

func (p *blockProcessor) createProof(unitID []byte, b *block.Block, tx *txsystem.Transaction) (*Proof, error) {
	if b == nil {
		return nil, nil
	}
	gblock, err := b.ToGenericBlock(p.txs)
	if err != nil {
		return nil, fmt.Errorf("failed to convert block to generic block: %w", err)
	}
	proof, err := block.NewPrimaryProof(gblock, unitID, crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary proof for the block: %w", err)
	}
	return &Proof{
		BlockNumber: b.UnicityCertificate.InputRecord.RoundNumber,
		Tx:          tx,
		Proof:       proof,
	}, nil
}
