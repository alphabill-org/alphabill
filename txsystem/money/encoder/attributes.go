package moneyenc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
)

func RegisterTxAttributeEncoders(reg func(id encoder.AttrEncID, enc encoder.TxAttributesEncoder) error) error {
	key := func(attrID string) encoder.AttrEncID {
		return encoder.AttrEncID{
			TxSys: money.DefaultSystemIdentifier,
			Attr:  attrID,
		}
	}
	return errors.Join(
		reg(key(money.PayloadTypeTransfer), txaTransferAttributes),
	)
}

/*
Registers all the encoders from RegisterTxAttributeEncoders for which the "filter" func returns "true".
*/
func RegisterTxAttributeEncodersF(filter func(encoder.AttrEncID) bool) func(func(encoder.AttrEncID, encoder.TxAttributesEncoder) error) error {
	return func(hostReg func(encoder.AttrEncID, encoder.TxAttributesEncoder) error) error {
		return RegisterTxAttributeEncoders(func(id encoder.AttrEncID, enc encoder.TxAttributesEncoder) error {
			if filter(id) {
				return hostReg(id, enc)
			}
			return nil
		})
	}
}

func txaTransferAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &money.TransferAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := make(encoder.WasmEnc, 0, 8+8)
	buf.WriteUInt64(attr.TargetValue)
	buf.WriteUInt64(attr.Counter)
	return buf, nil
}
