package tokens

import (
	"hash"

	"github.com/holiman/uint256"

	"github.com/alphabill-org/alphabill/internal/rma"
)

type nonFungibleTokenTypeData struct {
	symbol                   string
	parentTypeId             *uint256.Int // identifies the parent type that this type derives from; 0 indicates there is no parent type;
	subTypeCreationPredicate []byte       // the predicate clause that controls defining new sub-types of this type;
	tokenCreationPredicate   []byte       // the predicate clause that controls creating new tokens of this type
	invariantPredicate       []byte       // the invariant predicate clause that all tokens of this type (and of sub-types of this type) inherit into their bearer predicates;
	dataUpdatePredicate      []byte       // the clause that all tokens of this type (and of sub-types of this type) inherit into their data update predicates
}

func newNonFungibleTokenTypeData(tx *createNonFungibleTokenTypeWrapper) rma.UnitData {
	attr := tx.attributes
	return &nonFungibleTokenTypeData{
		symbol:                   attr.Symbol,
		parentTypeId:             tx.ParentTypeID(),
		subTypeCreationPredicate: attr.SubTypeCreationPredicate,
		tokenCreationPredicate:   attr.TokenCreationPredicate,
		invariantPredicate:       attr.InvariantPredicate,
		dataUpdatePredicate:      attr.DataUpdatePredicate,
	}
}

func (n *nonFungibleTokenTypeData) AddToHasher(hasher hash.Hash) {
	hasher.Write([]byte(n.symbol))
	hasher.Write(n.parentTypeId.Bytes())
	hasher.Write(n.subTypeCreationPredicate)
	hasher.Write(n.tokenCreationPredicate)
	hasher.Write(n.invariantPredicate)
	hasher.Write(n.dataUpdatePredicate)
}

// Value returns the SummaryValue of this single UnitData.
func (n *nonFungibleTokenTypeData) Value() rma.SummaryValue {
	return zeroSummaryValue
}
