package wvm

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
)

type handlerFunc func(obj any) uint64

type TXSystemEncoder struct{}

/*
Encode serializes well known types (not tx system specific) to WASM representation.
*/
func (enc TXSystemEncoder) Encode(obj any, getHandler handlerFunc) ([]byte, error) {
	switch t := obj.(type) {
	case *types.TransactionOrder:
		return enc.txOrder(t)
	case *types.TransactionRecord:
		return enc.txRecord(t, getHandler)
	case []byte:
		return t, nil
	}
	return nil, fmt.Errorf("no encoder for %T", obj)
}

func (TXSystemEncoder) txRecord(txo *types.TransactionRecord, getHandler func(obj any) uint64) ([]byte, error) {
	var buf wasmEnc
	buf.writeTypeVer(type_id_tx_record, 1)
	buf.writeUInt64(getHandler(txo.TransactionOrder))
	return buf, nil
}

func (TXSystemEncoder) txOrder(txo *types.TransactionOrder) ([]byte, error) {
	buf := make(wasmEnc, 0, (6*4)+len(txo.Payload.Type)+len(txo.Payload.UnitID)+len(txo.OwnerProof)+len(txo.FeeProof))

	buf.writeTypeVer(type_id_tx_order, 1)
	buf.writeUInt32(uint32(txo.SystemID()))
	buf.writeString(txo.Payload.Type)
	buf.writeBytes(txo.Payload.UnitID)
	buf.writeBytes(txo.OwnerProof)
	buf.writeBytes(txo.FeeProof)
	return buf, nil
}

func (TXSystemEncoder) TxAttributes(txo *types.TransactionOrder) ([]byte, error) {
	switch txo.Payload.SystemID {
	case tokens.DefaultSystemIdentifier:
		return TokenTXSEncoder{}.TxAttributes(txo)
	case money.DefaultSystemIdentifier:
		return MoneyTXSEncoder{}.TxAttributes(txo)
	}
	return nil, fmt.Errorf("serializing to bytes is not implemented for tx system %d %q attributes", txo.Payload.SystemID, txo.PayloadType())
}

func (TXSystemEncoder) UnitData(unit *state.Unit) ([]byte, error) {
	return TokenTXSEncoder{}.UnitData(unit)
}

type TokenTXSEncoder struct{}

func (TokenTXSEncoder) TxAttributes(txo *types.TransactionOrder) ([]byte, error) {
	switch txo.Payload.SystemID {
	case tokens.DefaultSystemIdentifier:
		switch txo.PayloadType() {
		case tokens.PayloadTypeCreateNFTType:
			attr := &tokens.CreateNonFungibleTokenTypeAttributes{}
			if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
				return nil, fmt.Errorf("reading tx attributes: %w", err)
			}
			buf := make(wasmEnc, 0, (3*4)+len(attr.Symbol)+len(attr.Name)+len(attr.ParentTypeID))
			buf.writeString(attr.Symbol)
			buf.writeString(attr.Name)
			buf.writeBytes(attr.ParentTypeID)
			return buf, nil
		case tokens.PayloadTypeMintNFT:
			attr := &tokens.MintNonFungibleTokenAttributes{}
			if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
				return nil, fmt.Errorf("reading tx attributes: %w", err)
			}
			buf := make(wasmEnc, 0)
			buf.writeString(attr.Name)
			buf.writeString(attr.URI)
			buf.writeBytes(attr.NFTTypeID)
			buf.writeBytes(attr.Data)
			return buf, nil
		case tokens.PayloadTypeTransferNFT:
			attr := &tokens.TransferNonFungibleTokenAttributes{}
			if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
				return nil, fmt.Errorf("reading tx attributes: %w", err)
			}
			buf := make(wasmEnc, 0)
			buf.writeBytes(attr.NFTTypeID)
			buf.writeBytes(attr.Nonce)
			buf.writeBytes(attr.Backlink)
			return buf, nil
		case tokens.PayloadTypeUpdateNFT:
			attr := &tokens.UpdateNonFungibleTokenAttributes{}
			if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
				return nil, fmt.Errorf("reading tx attributes: %w", err)
			}
			buf := make(wasmEnc, 0, 8+len(attr.Data)+len(attr.Backlink))
			buf.writeBytes(attr.Data)
			buf.writeBytes(attr.Backlink)
			return buf, nil
		}
		/*
			case tokens.PayloadTypeCreateFungibleTokenType:
				return &tokens.CreateFungibleTokenTypeAttributes{}, nil
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
	}
	return nil, fmt.Errorf("serializing to bytes is not implemented for tx system %d %q attributes", txo.Payload.SystemID, txo.PayloadType())
}

func (TokenTXSEncoder) UnitData(unit *state.Unit) ([]byte, error) {
	if unit == nil {
		return nil, errors.New("unit must be not nil")
	}

	var buf wasmEnc
	switch t := unit.Data().(type) {
	case *tokens.NonFungibleTokenData:
		buf.writeTypeVer(type_id_NFT_data, 1)
		buf.writeBytes(t.NftType)
		buf.writeString(t.Name)
		buf.writeString(t.URI)
		buf.writeBytes(t.Data)
		buf.writeUInt64(t.T)
		buf.writeBytes(t.Backlink)
		buf.writeUInt64(t.Locked)
	case *tokens.NonFungibleTokenTypeData:
		buf.writeTypeVer(type_id_NFT_type, 1)
		buf.writeBytes(t.ParentTypeId)
		buf.writeString(t.Symbol)
		buf.writeString(t.Name)
	default:
		return nil, fmt.Errorf("encoding unit data of %T is not implemeted", t)
	}
	return buf, nil
}

type MoneyTXSEncoder struct{}

func (MoneyTXSEncoder) TxAttributes(txo *types.TransactionOrder) ([]byte, error) {
	switch txo.Payload.SystemID {
	case money.DefaultSystemIdentifier:
		switch txo.PayloadType() {
		case money.PayloadTypeTransfer:
			attr := &money.TransferAttributes{}
			if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
				return nil, fmt.Errorf("reading tx attributes: %w", err)
			}
			buf := make(wasmEnc, 0, (8+4)+len(attr.Backlink))
			buf.writeUInt64(attr.TargetValue)
			buf.writeBytes(attr.Backlink)
			return buf, nil
		}
	}
	return nil, fmt.Errorf("serializing to bytes is not implemented for tx system %d %q attributes", txo.Payload.SystemID, txo.PayloadType())
}

type wasmEnc []byte

func (buf *wasmEnc) writeTypeVer(typID, ver uint16) {
	*buf = binary.LittleEndian.AppendUint32(*buf, (uint32(ver)<<16)+uint32(typID))
}

func (buf *wasmEnc) writeBytes(value []byte) {
	*buf = binary.LittleEndian.AppendUint32(*buf, uint32(len(value)))
	*buf = append(*buf, value...)
}

func (buf *wasmEnc) writeString(value string) {
	buf.writeBytes([]byte(value))
}

func (buf *wasmEnc) writeUInt64(value uint64) {
	*buf = binary.LittleEndian.AppendUint64(*buf, value)
}

func (buf *wasmEnc) writeUInt32(value uint32) {
	*buf = binary.LittleEndian.AppendUint32(*buf, value)
}
