package testutils

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

func NewFeeCreditRecordID(t *testing.T, signer abcrypto.Signer) types.UnitID {
	ownerPredicate := NewP2pkhPredicate(t, signer)
	ownerPredicateHash := hash.Sum256(ownerPredicate)
	return types.NewUnitID(money.UnitIDLength, nil, ownerPredicateHash, money.FeeCreditRecordUnitType)
}

func NewFeeCreditRecordIDAlwaysTrue() types.UnitID {
	ownerPredicate := templates.AlwaysTrueBytes()
	ownerPredicateHash := hash.Sum256(ownerPredicate)
	return types.NewUnitID(money.UnitIDLength, nil, ownerPredicateHash, money.FeeCreditRecordUnitType)
}

func NewP2pkhPredicate(t *testing.T, signer abcrypto.Signer) types.PredicateBytes {
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	publicKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	return templates.NewP2pkh256BytesFromKey(publicKey)
}

func DefaultMoneyUnitID() types.UnitID {
	return types.NewUnitID(money.UnitIDLength, nil, make([]byte, 32), money.BillUnitType)
}
