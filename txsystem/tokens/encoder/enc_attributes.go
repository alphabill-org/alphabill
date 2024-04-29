package tokenenc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
)

func RegisterTxAttributeEncoders(reg func(id encoder.AttrEncID, enc encoder.TxAttributesEncoder) error) error {
	key := func(attrID string) encoder.AttrEncID {
		return encoder.AttrEncID{
			TxSys: tokens.DefaultSystemIdentifier,
			Attr:  attrID,
		}
	}
	return errors.Join(
		reg(key(tokens.PayloadTypeCreateNFTType), txaCreateNonFungibleTokenTypeAttributes),
		reg(key(tokens.PayloadTypeMintNFT), txaMintNonFungibleTokenAttributes),
		reg(key(tokens.PayloadTypeTransferNFT), txaTransferNonFungibleTokenAttributes),
		reg(key(tokens.PayloadTypeUpdateNFT), txaUpdateNonFungibleTokenAttributes),
		reg(key(tokens.PayloadTypeCreateFungibleTokenType), txaCreateFungibleTokenTypeAttributes),
	)
}

func txaCreateNonFungibleTokenTypeAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.CreateNonFungibleTokenTypeAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	// alloc: 3*uint32 for lengths + data lengths
	buf := make(encoder.WasmEnc, 0, (3*4)+len(attr.Symbol)+len(attr.Name)+len(attr.ParentTypeID))
	buf.WriteString(attr.Symbol)
	buf.WriteString(attr.Name)
	buf.WriteBytes(attr.ParentTypeID)
	return buf, nil
}

func txaMintNonFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.MintNonFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := make(encoder.WasmEnc, 0)
	buf.WriteString(attr.Name)
	buf.WriteString(attr.URI)
	buf.WriteBytes(attr.Data)
	buf.WriteUInt64(attr.Nonce)
	return buf, nil
}

func txaTransferNonFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.TransferNonFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := make(encoder.WasmEnc, 0)
	buf.WriteBytes(attr.NFTTypeID)
	buf.WriteBytes(attr.Nonce)
	buf.WriteUInt64(attr.Counter)
	return buf, nil
}

func txaUpdateNonFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.UpdateNonFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := make(encoder.WasmEnc, 0, 8+len(attr.Data)+8)
	buf.WriteBytes(attr.Data)
	buf.WriteUInt64(attr.Counter)
	return buf, nil
}

func txaCreateFungibleTokenTypeAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	//attr := &tokens.CreateFungibleTokenTypeAttributes{}
	return nil, fmt.Errorf("not implemented")
}

/*
	case tokens.PayloadTypeMintFungibleToken:
		return &tokens.MintFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeTransferFungibleToken:
		return &tokens.TransferFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeSplitFungibleToken:
		return &tokens.SplitFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeBurnFungibleToken:
		return &tokens.BurnFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeJoinFungibleToken:
		return &tokens.JoinFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeLockToken:
		return &tokens.LockTokenAttributes{}, nil
	case tokens.PayloadTypeUnlockToken:
		return &tokens.UnlockTokenAttributes{}, nil
	}
*/
