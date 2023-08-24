package tokens

import (
	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	UnitIDLength   = UnitPartLength + TypePartLength
	UnitPartLength = 32
	TypePartLength = 1
)

var (
	FungibleTokenTypeUnitType    = []byte{0x01}
	FungibleTokenUnitType        = []byte{0x02}
	NonFungibleTokenTypeUnitType = []byte{0x11}
	NonFungibleTokenUnitType     = []byte{0x12}
	FeeCreditRecordUnitType      = []byte{0xff}
)

func NewFungibleTokenTypeID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, FungibleTokenTypeUnitType)
}

func NewFungibleTokenID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, FungibleTokenUnitType)
}

func NewNonFungibleTokenTypeID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, NonFungibleTokenTypeUnitType)
}

func NewNonFungibleTokenID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, NonFungibleTokenUnitType)
}

func NewFeeCreditRecordID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, FeeCreditRecordUnitType)
}
