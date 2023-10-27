package money

import (
	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/common/hash"
	"github.com/alphabill-org/alphabill/txsystem/money"
)

func FeeCreditRecordIDFormPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := hash.Sum256(pubKey)
	return money.NewFeeCreditRecordID(shardPart, unitPart)
}
