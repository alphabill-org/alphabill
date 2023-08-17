package money

import (
	"github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	TypeIDLength = 1
	UnitIDLength = 32 + TypeIDLength
)

var (
	BillTypeID            = []byte{1}
	FeeCreditRecordTypeID = []byte{2}
)

func SameShardID(unitID types.UnitID, hashValue []byte) types.UnitID {
	return util.SameShardID(unitID, hashValue, TypeIDLength)
}
