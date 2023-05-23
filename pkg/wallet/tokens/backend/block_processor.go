package backend

import (
	"bytes"
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/broker"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type blockProcessor struct {
	store  Storage
	txs    txsystem.TransactionSystem
	notify func(bearerPredicate []byte, msg broker.Message)
	log    log.Logger
}

func (p *blockProcessor) ProcessBlock(ctx context.Context, b *types.Block) error {
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

func (p *blockProcessor) processTx(tr *types.TransactionRecord, b *types.Block) error {
	tx := tr.TransactionOrder
	id := tx.UnitID()
	txHash := tx.Hash(crypto.SHA256)
	proof, err := p.createProof(id, b, tx)
	if err != nil {
		return fmt.Errorf("failed to create proof for tx with id=%X: %w", txHash, err)
	}

	p.log.Debug(fmt.Sprintf("processTx: UnitID=%x type: %s", id, tx.PayloadType()))

	// handle fee credit txs
	switch tx.Payload.Type {
	case transactions.PayloadTypeAddFeeCredit:
		addFeeCreditAttributes := &transactions.AddFeeCreditAttributes{}
		if err = tx.UnmarshalAttributes(addFeeCreditAttributes); err != nil {
			return err
		}
		transferFeeCreditAttributes := &transactions.TransferFeeCreditAttributes{}
		if err = addFeeCreditAttributes.FeeCreditTransfer.TransactionOrder.UnmarshalAttributes(transferFeeCreditAttributes); err != nil {
			return err
		}
		fcb, err := p.store.GetFeeCreditBill(id)
		if err != nil {
			return err
		}
		return p.store.SetFeeCreditBill(&FeeCreditBill{
			Id:            id,
			Value:         fcb.GetValue() + transferFeeCreditAttributes.Amount - tr.ServerMetadata.ActualFee,
			TxHash:        tx.Hash(crypto.SHA256),
			FCBlockNumber: b.GetRoundNumber(),
		}, proof)
	case transactions.PayloadTypeCloseFeeCredit:
		closeFeeCreditAttributes := &transactions.CloseFeeCreditAttributes{}
		if err = tx.UnmarshalAttributes(closeFeeCreditAttributes); err != nil {
			return err
		}
		fcb, err := p.store.GetFeeCreditBill(id)
		if err != nil {
			return err
		}
		return p.store.SetFeeCreditBill(&FeeCreditBill{
			Id:            id,
			Value:         fcb.GetValue() - closeFeeCreditAttributes.Amount,
			TxHash:        tx.Hash(crypto.SHA256),
			FCBlockNumber: b.GetRoundNumber(),
		}, proof)
	default:
		// decrement fee credit bill value if tx is not fee credit tx i.e. a normal tx
		if err := p.updateFCB(tx, b.GetRoundNumber()); err != nil {
			return fmt.Errorf("failed to update fee credit bill %w", err)
		}
	}

	// handle UT transactions
	switch tx.Payload.Type {
	case tokens.PayloadTypeCreateFungibleTokenType:
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
	case tokens.PayloadTypeMintFungibleToken:
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
	case tokens.PayloadTypeTransferFungibleToken:
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("fungible transfer tx: failed to get token with id=%X: %w", id, err)
		}
		token.TxHash = txHash
		token.Owner = tx.NewBearer()
		return p.saveToken(token, proof)
	case tokens.PayloadTypeSplitFungibleToken:
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
		splitProof, err := p.createProof(newId, b, tx)
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
	case tokens.PayloadTypeBurnFungibleToken:
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
	case tokens.PayloadTypeJoinFungibleToken:
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
	case tokens.PayloadTypeCreateNFTType:
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
	case tokens.PayloadTypeMintNFT:
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
	case tokens.PayloadTypeTransferNFT:
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("transfer nft tx: failed to get token with id=%X: %w", id, err)
		}
		token.Owner = tx.NewBearer()
		token.TxHash = txHash
		return p.saveToken(token, proof)
	case tokens.PayloadTypeUpdateNFT:
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

func (p *blockProcessor) saveTokenType(unit *TokenUnitType, proof *wallet.Proof) error {
	if err := p.store.SaveTokenType(unit, proof); err != nil {
		return fmt.Errorf("failed to store token type: %w", err)
	}
	return nil
}

func (p *blockProcessor) saveToken(unit *TokenUnit, proof *wallet.Proof) error {
	if err := p.store.SaveToken(unit, proof); err != nil {
		return fmt.Errorf("failed to store token: %w", err)
	}
	p.notify(unit.Owner, unit)
	return nil
}

func (p *blockProcessor) createProof(unitID wallet.UnitID, b *block.Block, tx *txsystem.Transaction) (*wallet.Proof, error) {
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
	return &wallet.Proof{
		BlockNumber: b.UnicityCertificate.InputRecord.RoundNumber,
		Tx:          tx,
		Proof:       proof,
	}, nil
}

func (p *blockProcessor) updateFCB(tx *txsystem.Transaction, roundNumber uint64) error {
	fcb, err := p.store.GetFeeCreditBill(tx.ClientMetadata.FeeCreditRecordId)
	if err != nil {
		return err
	}
	if fcb == nil {
		return fmt.Errorf("fee credit bill not found: %X", tx.ClientMetadata.FeeCreditRecordId)
	}
	if fcb.Value < tx.ServerMetadata.Fee {
		return fmt.Errorf("insufficient fee credit - fee is %d but remaining credit is only %d", tx.ServerMetadata.Fee, fcb.Value)
	}
	fcb.Value -= tx.ServerMetadata.Fee
	fcb.FCBlockNumber = roundNumber
	return p.store.SetFeeCreditBill(fcb, nil)
}
