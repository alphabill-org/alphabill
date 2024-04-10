package moneyenc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wvm/encoder"
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

func txaTransferAttributes(txo *types.TransactionOrder) ([]byte, error) {
	attr := &money.TransferAttributes{}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading tx attributes: %w", err)
	}
	buf := make(encoder.WasmEnc, 0, 8+4+8)
	buf.WriteUInt64(attr.TargetValue)
	buf.WriteUInt64(attr.Counter)
	return buf, nil
}
