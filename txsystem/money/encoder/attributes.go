package moneyenc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
)

func RegisterTxAttributeEncoders(reg func(id encoder.PartitionTxType, enc encoder.TxAttributesEncoder) error) error {
	key := func(attrID uint16) encoder.PartitionTxType {
		return encoder.PartitionTxType{
			Partition: money.DefaultPartitionID,
			TxType:    attrID,
		}
	}
	return errors.Join(
		reg(key(money.TransactionTypeTransfer), txaTransferAttributes),
	)
}

/*
Registers all the encoders from RegisterTxAttributeEncoders for which the "filter" func returns "true".
*/
func RegisterTxAttributeEncodersF(filter func(encoder.PartitionTxType) bool) func(func(encoder.PartitionTxType, encoder.TxAttributesEncoder) error) error {
	return func(hostReg func(encoder.PartitionTxType, encoder.TxAttributesEncoder) error) error {
		return RegisterTxAttributeEncoders(func(id encoder.PartitionTxType, enc encoder.TxAttributesEncoder) error {
			if filter(id) {
				return hostReg(id, enc)
			}
			return nil
		})
	}
}

func txaTransferAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	attr := &money.TransferAttributes{}
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("reading transaction attributes: %w", err)
	}
	buf := encoder.TVEnc{}
	buf.EncodeTagged(1, attr.TargetValue)
	buf.EncodeTagged(2, attr.Counter)
	return buf.Bytes()
}
