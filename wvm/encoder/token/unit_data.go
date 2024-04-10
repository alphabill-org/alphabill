package tokenenc

import (
	"errors"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/wvm/encoder"
)

func RegisterUnitDataEncoders(reg func(ud any, enc encoder.UnitDataEncoder) error) error {
	return errors.Join(
		reg(&tokens.NonFungibleTokenData{}, udeNonFungibleTokenData),
		reg(&tokens.NonFungibleTokenTypeData{}, udeNonFungibleTokenTypeData),
	)
}

func udeNonFungibleTokenData(data state.UnitData) ([]byte, error) {
	value := data.(*tokens.NonFungibleTokenData)
	var buf encoder.WasmEnc
	buf.WriteTypeVer(type_id_NFT_data, 1)
	buf.WriteBytes(value.NftType)
	buf.WriteString(value.Name)
	buf.WriteString(value.URI)
	buf.WriteBytes(value.Data)
	buf.WriteUInt64(value.T)
	buf.WriteUInt64(value.Counter)
	buf.WriteUInt64(value.Locked)
	return buf, nil
}

func udeNonFungibleTokenTypeData(data state.UnitData) ([]byte, error) {
	value := data.(*tokens.NonFungibleTokenTypeData)
	var buf encoder.WasmEnc
	buf.WriteTypeVer(type_id_NFT_type, 1)
	buf.WriteBytes(value.ParentTypeId)
	buf.WriteString(value.Symbol)
	buf.WriteString(value.Name)
	return buf, nil
}

const (
	type_id_NFT_data = 5
	type_id_NFT_type = 6
)
