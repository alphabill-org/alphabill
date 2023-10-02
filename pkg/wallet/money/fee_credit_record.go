package money

import (
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
)

func FeeCreditRecordIDFormPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := hash.Sum256(pubKey)
	return money.NewFeeCreditRecordID(shardPart, unitPart)
}
