package tokens

import (
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
)

func FeeCreditRecordIDFromPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := hash.Sum256(pubKey)
	return tokens.NewFeeCreditRecordID(shardPart, unitPart)
}
