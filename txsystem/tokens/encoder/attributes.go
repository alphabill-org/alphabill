package tokenenc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
)

func RegisterTxAttributeEncoders(reg func(id encoder.AttrEncID, enc encoder.TxAttributesEncoder) error) error {
	key := func(attrID uint16) encoder.AttrEncID {
		return encoder.AttrEncID{
			TxSys: tokens.DefaultPartitionID,
			Attr:  attrID,
		}
	}
	return errors.Join(
		reg(key(tokens.TransactionTypeDefineNFT), txaCreateNonFungibleTokenTypeAttributes),
		reg(key(tokens.TransactionTypeMintNFT), txaMintNonFungibleTokenAttributes),
		reg(key(tokens.TransactionTypeTransferNFT), txaTransferNonFungibleTokenAttributes),
		reg(key(tokens.TransactionTypeUpdateNFT), txaUpdateNonFungibleTokenAttributes),
		reg(key(tokens.TransactionTypeDefineFT), txaDefineFungibleTokenAttributes),
		reg(key(tokens.TransactionTypeMintFT), txaMintFungibleTokenAttributes),
		reg(key(tokens.TransactionTypeTransferFT), txaTransferFungibleTokenAttributes),
		reg(key(tokens.TransactionTypeSplitFT), txaSplitFungibleTokenAttributes),
		reg(key(tokens.TransactionTypeBurnFT), txaBurnFungibleTokenAttributes),
		reg(key(tokens.TransactionTypeJoinFT), txaJoinFungibleTokenAttributes),
		reg(key(tokens.TransactionTypeLockToken), txaLockTokenAttributes),
		reg(key(tokens.TransactionTypeUnlockToken), txaUnlockTokenAttributes),
	)
}

func txaCreateNonFungibleTokenTypeAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.DefineNonFungibleTokenAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
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
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
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
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TypeID)
	buf.EncodeTagged(2, attr.Counter)
	return buf.Bytes()
}

func txaUpdateNonFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.UpdateNonFungibleTokenAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.Data)
	buf.EncodeTagged(2, attr.Counter)
	return buf.Bytes()
}

func txaDefineFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.DefineFungibleTokenAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.Symbol)
	buf.EncodeTagged(2, attr.Name)
	if len(attr.ParentTypeID) != 0 {
		buf.EncodeTagged(3, attr.ParentTypeID)
	}
	buf.EncodeTagged(4, attr.DecimalPlaces)
	return buf.Bytes()
}

func txaMintFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.MintFungibleTokenAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TypeID)
	buf.EncodeTagged(2, attr.Value)
	buf.EncodeTagged(3, attr.Nonce)
	return buf.Bytes()
}

func txaTransferFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.TransferFungibleTokenAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TypeID)
	buf.EncodeTagged(2, attr.Value)
	buf.EncodeTagged(3, attr.Counter)
	return buf.Bytes()
}

func txaSplitFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.SplitFungibleTokenAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TypeID)
	buf.EncodeTagged(2, attr.Counter)
	buf.EncodeTagged(3, attr.TargetValue)
	return buf.Bytes()
}

func txaBurnFungibleTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.BurnFungibleTokenAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
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
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	// register and then return handles of the txR and proofs?
	return buf.Bytes()
}

func txaLockTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.LockTokenAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.Counter)
	buf.EncodeTagged(2, attr.LockStatus)
	return buf.Bytes()
}

func txaUnlockTokenAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &tokens.UnlockTokenAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.Counter)
	return buf.Bytes()
}
