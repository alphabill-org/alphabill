package twb

import (
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"

	wtokens "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
)

type blockProcessor struct {
	store Storage
	txs   txsystem.TransactionSystem
}

func (p *blockProcessor) ProcessBlock(ctx context.Context, b *block.Block) error {
	lastBlockNumber, err := p.store.GetBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to read current block number: %w", err)
	}
	if b.BlockNumber != lastBlockNumber+1 {
		return fmt.Errorf("invalid block height. Received blockNumber %d current wallet blockNumber %d", b.BlockNumber, lastBlockNumber)
	}

	for _, tx := range b.Transactions {
		if err := p.processTx(tx, b); err != nil {
			return fmt.Errorf("failed to process tx: %w", err)
		}
	}

	return p.store.SetBlockNumber(b.BlockNumber)
}

func (p *blockProcessor) processTx(ptx *txsystem.Transaction, b *block.Block) error {
	gtx, err := p.txs.ConvertTx(ptx)
	if err != nil {
		return fmt.Errorf("failed to convert to generic tx: %w", err)
	}

	switch gtx.(type) {
	case tokens.CreateFungibleTokenType, tokens.CreateNonFungibleTokenType:
		return p.saveTokenTypeTx(gtx)
	case tokens.MintFungibleToken, tokens.TransferFungibleToken:
		proof, err := p.createProof(b, ptx)
		if err != nil {
			return fmt.Errorf("failed to create proof for tx: %w", err)
		}
		return p.saveFungibleTokenTx(gtx, proof)
	case tokens.MintNonFungibleToken, tokens.TransferNonFungibleToken:
		proof, err := p.createProof(b, ptx)
		if err != nil {
			return fmt.Errorf("failed to create proof for tx: %w", err)
		}
		return p.saveNonFungibleTokenTx(gtx, proof)
	}

	return fmt.Errorf("not implemented for tx (sys:%x  unit:%x)", ptx.SystemId, ptx.UnitId)
}

func (p *blockProcessor) createProof(b *block.Block, tx *txsystem.Transaction) (*wtokens.Proof, error) {
	if b == nil {
		return nil, nil
	}
	gblock, err := b.ToGenericBlock(p.txs)
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

func (p *blockProcessor) saveNonFungibleTokenTx(tx txsystem.GenericTransaction, proof *wtokens.Proof) error {
	type nfTokenTx interface {
		NFTTypeID() []byte
	}
	ttx := tx.(nfTokenTx)

	tType, err := p.store.GetTokenType(ttx.NFTTypeID())
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
	if err := p.store.SaveTokenUnit(d); err != nil {
		return fmt.Errorf("failed to save %s (%x): %w", d.Kind, d.ID, err)
	}
	return nil
}

func (p *blockProcessor) saveFungibleTokenTx(tx txsystem.GenericTransaction, proof *wtokens.Proof) error {
	type fungibleTokenTx interface {
		TypeID() []byte
		Value() uint64
	}
	ttx := tx.(fungibleTokenTx)

	tType, err := p.store.GetTokenType(ttx.TypeID())
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
	if err := p.store.SaveTokenUnit(d); err != nil {
		return fmt.Errorf("failed to save %s (%x): %w", d.Kind, d.ID, err)
	}
	return nil
}

func (p *blockProcessor) saveTokenTypeTx(tx txsystem.GenericTransaction) error {
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
	if err := p.store.SaveTokenType(d); err != nil {
		return fmt.Errorf("failed to save %s (%x): %w", d.Kind, d.ID, err)
	}
	return nil
}
