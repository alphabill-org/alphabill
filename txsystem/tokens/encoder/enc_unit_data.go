package tokenenc

import (
	"errors"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
)

func RegisterUnitDataEncoders(reg func(ud any, enc encoder.UnitDataEncoder) error) error {
	return errors.Join(
		reg(&tokens.NonFungibleTokenData{}, udeNonFungibleTokenData),
		reg(&tokens.NonFungibleTokenTypeData{}, udeNonFungibleTokenTypeData),
	)
}

func udeNonFungibleTokenData(data types.UnitData, ver uint32) ([]byte, error) {
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

func udeNonFungibleTokenTypeData(data types.UnitData, ver uint32) ([]byte, error) {
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
