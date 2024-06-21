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
		reg(key(tokens.PayloadTypeMintFungibleToken), txaMintFungibleTokenAttributes),
		reg(key(tokens.PayloadTypeTransferFungibleToken), txaTransferFungibleTokenAttributes),
		reg(key(tokens.PayloadTypeSplitFungibleToken), txaSplitFungibleTokenAttributes),
		reg(key(tokens.PayloadTypeBurnFungibleToken), txaBurnFungibleTokenAttributes),
		reg(key(tokens.PayloadTypeJoinFungibleToken), txaJoinFungibleTokenAttributes),
		reg(key(tokens.PayloadTypeLockToken), txaLockTokenAttributes),
		reg(key(tokens.PayloadTypeUnlockToken), txaUnlockTokenAttributes),
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
	attr := &tokens.CreateFungibleTokenTypeAttributes{}
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

func txaMintFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.MintFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TypeID)
	buf.EncodeTagged(2, attr.Value)
	buf.EncodeTagged(3, attr.Nonce)
	return buf.Bytes()
}

func txaTransferFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.TransferFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TypeID)
	buf.EncodeTagged(2, attr.Value)
	buf.EncodeTagged(3, attr.Nonce)
	buf.EncodeTagged(4, attr.Counter)
	return buf.Bytes()
}

func txaSplitFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.SplitFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TypeID)
	buf.EncodeTagged(2, attr.RemainingValue)
	buf.EncodeTagged(3, attr.Nonce)
	buf.EncodeTagged(4, attr.Counter)
	buf.EncodeTagged(5, attr.TargetValue)
	return buf.Bytes()
}

func txaBurnFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.BurnFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TypeID)
	buf.EncodeTagged(2, attr.Value)
	buf.EncodeTagged(3, attr.Counter)
	buf.EncodeTagged(4, attr.TargetTokenID)
	buf.EncodeTagged(5, attr.TargetTokenCounter)
	return buf.Bytes()
}

func txaJoinFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.JoinFungibleTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	// register and then return handles of the txR and proofs?
	return buf.Bytes()
}

func txaLockTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.LockTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.Counter)
	buf.EncodeTagged(2, attr.LockStatus)
	return buf.Bytes()
}

func txaUnlockTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.UnlockTokenAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.Counter)
	return buf.Bytes()
}
