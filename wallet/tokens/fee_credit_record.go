package tokens

import (
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

func FeeCreditRecordIDFromPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := util.Sum256(pubKey)
	return tokens.NewFeeCreditRecordID(shardPart, unitPart)
}
