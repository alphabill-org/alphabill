package tokens

import (
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

type nonFungibleTokenTypeData struct {
	symbol                   string
	parentTypeId             *uint256.Int // identifies the parent type that this type derives from; 0 indicates there is no parent type;
	subTypeCreationPredicate []byte       // the predicate clause that controls defining new sub-types of this type;
	tokenCreationPredicate   []byte       // the predicate clause that controls creating new tokens of this type
	invariantPredicate       []byte       // the invariant predicate clause that all tokens of this type (and of sub-types of this type) inherit into their bearer predicates;
	dataUpdatePredicate      []byte       // the clause that all tokens of this type (and of sub-types of this type) inherit into their data update predicates
}

type fungibleTokenTypeData struct {
	symbol                   string
	parentTypeId             *uint256.Int // identifies the parent type that this type derives from; 0 indicates there is no parent type;
	decimalPlaces            uint32       // is the number of decimal places to display for values of tokens of this type;
	subTypeCreationPredicate []byte       // the predicate clause that controls defining new sub-types of this type;
	tokenCreationPredicate   []byte       // the predicate clause that controls creating new tokens of this type
	invariantPredicate       []byte       // the invariant predicate clause that all tokens of this type (and of sub-types of this type) inherit into their bearer predicates;
}

type nonFungibleTokenData struct {
	nftType             *uint256.Int
	uri                 string // uri is the optional URI of an external resource associated with the token
	data                []byte // data is the optional data associated with the token.
	dataUpdatePredicate []byte // the data update predicate;
	t                   uint64 // the round number of the last transaction with this token;
	backlink            []byte // the hash of the last transaction order for this token
}

type fungibleTokenData struct {
	tokenType *uint256.Int // the type of the token;
	value     uint64       // the value of the token;
	t         uint64       // the round number of the last transaction with this token;
	backlink  []byte       // the hash of the last transaction order for this token
}

func newFungibleTokenTypeData(tx *createFungibleTokenTypeWrapper) rma.UnitData {
	attr := tx.attributes
	return &fungibleTokenTypeData{
		symbol:                   attr.Symbol,
		parentTypeId:             tx.ParentTypeID(),
		decimalPlaces:            attr.DecimalPlaces,
		subTypeCreationPredicate: attr.SubTypeCreationPredicate,
		tokenCreationPredicate:   attr.TokenCreationPredicate,
		invariantPredicate:       attr.InvariantPredicate,
	}
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

func newNonFungibleTokenData(tx *mintNonFungibleTokenWrapper, hasher crypto.Hash) rma.UnitData {
	attr := tx.attributes
	return &nonFungibleTokenData{
		nftType:             tx.NFTTypeID(),
		uri:                 attr.Uri,
		data:                attr.Data,
		dataUpdatePredicate: attr.DataUpdatePredicate,
		t:                   0,                           // we don't have previous tx
		backlink:            make([]byte, hasher.Size()), // in case of new NFT token the backlink is zero hash
	}
}

func newFungibleTokenData(tx *mintFungibleTokenWrapper, hasher crypto.Hash) rma.UnitData {
	attr := tx.attributes
	return &fungibleTokenData{
		tokenType: tx.TypeID(),
		value:     attr.Value,
		t:         0,                           // we don't have previous tx
		backlink:  make([]byte, hasher.Size()), // in case of new NFT token the backlink is zero hash
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

func (n *nonFungibleTokenData) AddToHasher(hasher hash.Hash) {
	hasher.Write(n.nftType.Bytes())
	hasher.Write([]byte(n.uri))
	hasher.Write(n.data)
	hasher.Write(n.dataUpdatePredicate)
	hasher.Write(util.Uint64ToBytes(n.t))
	hasher.Write(n.backlink)
}

func (n *nonFungibleTokenData) Value() rma.SummaryValue {
	return zeroSummaryValue
}

func (f *fungibleTokenTypeData) AddToHasher(hasher hash.Hash) {
	hasher.Write([]byte(f.symbol))
	hasher.Write(f.parentTypeId.Bytes())
	hasher.Write(util.Uint32ToBytes(f.decimalPlaces))
	hasher.Write(f.subTypeCreationPredicate)
	hasher.Write(f.tokenCreationPredicate)
	hasher.Write(f.invariantPredicate)
}

func (f *fungibleTokenTypeData) Value() rma.SummaryValue {
	return zeroSummaryValue
}

func (f *fungibleTokenData) AddToHasher(hasher hash.Hash) {
	hasher.Write(f.tokenType.Bytes())
	hasher.Write(util.Uint64ToBytes(f.value))
	hasher.Write(util.Uint64ToBytes(f.t))
	hasher.Write(f.backlink)
}

func (f *fungibleTokenData) Value() rma.SummaryValue {
	return zeroSummaryValue
}
