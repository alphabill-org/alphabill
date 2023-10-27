package tokens

import (
	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/common/hash"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
)

func FeeCreditRecordIDFromPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := hash.Sum256(pubKey)
	return tokens.NewFeeCreditRecordID(shardPart, unitPart)
}
