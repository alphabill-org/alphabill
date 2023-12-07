package money

import (
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

func FeeCreditRecordIDFormPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := util.Sum256(pubKey)
	return money.NewFeeCreditRecordID(shardPart, unitPart)
}
