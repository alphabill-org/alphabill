package twb

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"strings"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type blockProcessor struct {
	store Storage
	txs   txsystem.TransactionSystem
	log   log.Logger
}

func (p *blockProcessor) ProcessBlock(ctx context.Context, b *block.Block) error {
	lastBlockNumber, err := p.store.GetBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to read current block number: %w", err)
	}
	// block numbers must not be sequential (gaps might appear as empty block are not stored
	// and sent) but must be in ascending order
	if lastBlockNumber >= b.UnicityCertificate.InputRecord.RoundNumber {
		return fmt.Errorf("invalid block, received block %d, current wallet block %d", b.UnicityCertificate.InputRecord.RoundNumber, lastBlockNumber)
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

	p.log.Debug(fmt.Sprintf("processTx: UnitID=%x type: %s", id, strings.TrimPrefix(inTx.GetTransactionAttributes().TypeUrl, "type.googleapis.com/alphabill.tokens.v1.")))
	switch tx := gtx.(type) {
	case tokens.CreateFungibleTokenType:
		return p.saveTokenType(&TokenUnitType{
			Kind:                     Fungible,
			ID:                       id,
			ParentTypeID:             tx.ParentTypeID(),
			Symbol:                   tx.Symbol(),
			Name:                     tx.Name(),
			Icon:                     tx.Icon(),
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
	case tokens.BurnFungibleToken:
		token, err := p.store.GetToken(id)
		if err != nil {
			return err
		}
		if token.Amount != tx.Value() {
			return fmt.Errorf("expected burned amount: %v, got %v. token id='%X', type id='%X'", token.Amount, tx.Value(), token.ID, token.TypeID)
		}
		token.TxHash = txHash
		token.Burned = true
		return p.saveToken(token, proof)
	case tokens.JoinFungibleToken:
		joinedToken, err := p.store.GetToken(id)
		if err != nil {
			return err
		}
		if joinedToken == nil {
			return nil
		}
		burnedTokensToRemove := make([]TokenID, 0, len(tx.BurnTransactions()))
		var burnedValue uint64
		for _, burnTx := range tx.BurnTransactions() {
			burnedID := util.Uint256ToBytes(burnTx.UnitID())
			burnedToken, err := p.store.GetToken(burnedID)
			if err != nil {
				return err
			}
			if !burnedToken.Burned {
				return fmt.Errorf("token with id '%X' is expected to be burned, but it is not", burnedID)
			}
			if !bytes.Equal(burnedToken.Owner, joinedToken.Owner) {
				return fmt.Errorf("expected burned token's bearer '%X', got %X", joinedToken.Owner, burnedToken.Owner)
			}
			if !bytes.Equal(joinedToken.TxHash, burnTx.Nonce()) {
				return fmt.Errorf("expected burned token's nonce '%X', got %X", joinedToken.TxHash, burnTx.Nonce())
			}
			burnedTokensToRemove = append(burnedTokensToRemove, burnedID)
			burnedValue += burnTx.Value()
		}
		joinedToken.Amount += burnedValue
		joinedToken.TxHash = txHash
		if err = p.saveToken(joinedToken, proof); err != nil {
			return fmt.Errorf("failed to save joined token: %w", err)
		}
		for _, burnedID := range burnedTokensToRemove {
			if err = p.store.RemoveToken(burnedID); err != nil {
				return fmt.Errorf("failed to remove burned token %X: %w", burnedID, err)
			}
		}
		return nil
	case tokens.CreateNonFungibleTokenType:
		return p.saveTokenType(&TokenUnitType{
			Kind:                     NonFungible,
			ID:                       id,
			ParentTypeID:             tx.ParentTypeID(),
			Symbol:                   tx.Symbol(),
			Name:                     tx.Name(),
			Icon:                     tx.Icon(),
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
			NftName:                tx.Name(),
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
		token.TxHash = txHash
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
		p.log.Error("received unknown token transaction type, skipped processing:", fmt.Sprintf("data type: %T", tx))
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

func (p *blockProcessor) createProof(unitID UnitID, b *block.Block, tx *txsystem.Transaction) (*Proof, error) {
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
