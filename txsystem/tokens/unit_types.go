package tokens

import (
	"crypto/rand"

	"github.com/alphabill-org/alphabill/types"
)

const (
	UnitIDLength   = UnitPartLength + TypePartLength
	UnitPartLength = 32
	TypePartLength = 1
)

var (
	FungibleTokenTypeUnitType    = []byte{0x20}
	FungibleTokenUnitType        = []byte{0x21}
	NonFungibleTokenTypeUnitType = []byte{0x22}
	NonFungibleTokenUnitType     = []byte{0x23}
	FeeCreditRecordUnitType      = []byte{0x2f}
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

func NewRandomFungibleTokenTypeID(shardPart []byte) (types.UnitID, error) {
	return newRandomUnitID(shardPart, FungibleTokenTypeUnitType)
}

func NewRandomFungibleTokenID(shardPart []byte) (types.UnitID, error) {
	return newRandomUnitID(shardPart, FungibleTokenUnitType)
}

func NewRandomNonFungibleTokenTypeID(shardPart []byte) (types.UnitID, error) {
	return newRandomUnitID(shardPart, NonFungibleTokenTypeUnitType)
}

func NewRandomNonFungibleTokenID(shardPart []byte) (types.UnitID, error) {
	return newRandomUnitID(shardPart, NonFungibleTokenUnitType)
}

func newRandomUnitID(shardPart []byte, typePart []byte) (types.UnitID, error) {
	unitPart := make([]byte, UnitPartLength)
	_, err := rand.Read(unitPart)
	if err != nil {
		return nil, err
	}
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, typePart), nil
}
