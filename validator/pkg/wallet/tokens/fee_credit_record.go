package tokens

import (
	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/validator/internal/hash"
)

func FeeCreditRecordIDFromPublicKey(shardPart, pubKey []byte) types.UnitID {
	unitPart := hash.Sum256(pubKey)
	return tokens.NewFeeCreditRecordID(shardPart, unitPart)
}
