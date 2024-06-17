package tokenenc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
)

func RegisterTxAttributeEncoders(reg func(id encoder.AttrEncID, enc encoder.TxAttributesEncoder) error) error {
	key := func(attrID string) encoder.AttrEncID {
		return encoder.AttrEncID{
			TxSys: tokens.DefaultSystemID,
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
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.Symbol)
	buf.EncodeTagged(2, attr.Name)
	if len(attr.ParentTypeID) != 0 {
		buf.EncodeTagged(3, attr.ParentTypeID)
	}
	return buf.Bytes()
}

func txaMintNonFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.MintNonFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.Name)
	if attr.URI != "" {
		buf.EncodeTagged(2, attr.URI)
	}
	if len(attr.Data) != 0 {
		buf.EncodeTagged(3, attr.Data)
	}
	buf.EncodeTagged(4, attr.Nonce)
	buf.EncodeTagged(5, attr.TypeID)
	return buf.Bytes()
}

func txaTransferNonFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.TransferNonFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TypeID)
	if len(attr.Nonce) != 0 {
		buf.EncodeTagged(2, attr.Nonce)
	}
	buf.EncodeTagged(3, attr.Counter)
	return buf.Bytes()
}

func txaUpdateNonFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.UpdateNonFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.Data)
	buf.EncodeTagged(2, attr.Counter)
	return buf.Bytes()
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
