package tokens

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

type nonFungibleTokenTypeData struct {
	symbol                   string
	name                     string
	icon                     *Icon
	parentTypeId             *uint256.Int // identifies the parent type that this type derives from; 0 indicates there is no parent type;
	subTypeCreationPredicate []byte       // the predicate clause that controls defining new sub-types of this type;
	tokenCreationPredicate   []byte       // the predicate clause that controls creating new tokens of this type
	invariantPredicate       []byte       // the invariant predicate clause that all tokens of this type (and of sub-types of this type) inherit into their bearer predicates;
	dataUpdatePredicate      []byte       // the clause that all tokens of this type (and of sub-types of this type) inherit into their data update predicates
}

type fungibleTokenTypeData struct {
	symbol                   string
	name                     string
	icon                     *Icon
	parentTypeId             *uint256.Int // identifies the parent type that this type derives from; 0 indicates there is no parent type;
	decimalPlaces            uint32       // is the number of decimal places to display for values of tokens of this type;
	subTypeCreationPredicate []byte       // the predicate clause that controls defining new sub-types of this type;
	tokenCreationPredicate   []byte       // the predicate clause that controls creating new tokens of this type
	invariantPredicate       []byte       // the invariant predicate clause that all tokens of this type (and of sub-types of this type) inherit into their bearer predicates;
}

type nonFungibleTokenData struct {
	nftType             *uint256.Int
	name                string // the optional long name of the token
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

func newFungibleTokenTypeData(attr *CreateFungibleTokenTypeAttributes) rma.UnitData {
	return &fungibleTokenTypeData{
		symbol:                   attr.Symbol,
		name:                     attr.Name,
		icon:                     attr.Icon,
		parentTypeId:             util.BytesToUint256(attr.ParentTypeID),
		decimalPlaces:            attr.DecimalPlaces,
		subTypeCreationPredicate: attr.SubTypeCreationPredicate,
		tokenCreationPredicate:   attr.TokenCreationPredicate,
		invariantPredicate:       attr.InvariantPredicate,
	}
}

func newNonFungibleTokenTypeData(attr *CreateNonFungibleTokenTypeAttributes) rma.UnitData {
	return &nonFungibleTokenTypeData{
		symbol:                   attr.Symbol,
		name:                     attr.Name,
		icon:                     attr.Icon,
		parentTypeId:             util.BytesToUint256(attr.ParentTypeID),
		subTypeCreationPredicate: attr.SubTypeCreationPredicate,
		tokenCreationPredicate:   attr.TokenCreationPredicate,
		invariantPredicate:       attr.InvariantPredicate,
		dataUpdatePredicate:      attr.DataUpdatePredicate,
	}
}

func newNonFungibleTokenData(attr *MintNonFungibleTokenAttributes, txHash []byte, currentBlockNr uint64) rma.UnitData {
	return &nonFungibleTokenData{
		nftType:             util.BytesToUint256(attr.NFTTypeID),
		name:                attr.Name,
		uri:                 attr.URI,
		data:                attr.Data,
		dataUpdatePredicate: attr.DataUpdatePredicate,
		t:                   currentBlockNr,
		backlink:            txHash,
	}
}

func newFungibleTokenData(attr *MintFungibleTokenAttributes, txHash []byte, currentBlockNr uint64) rma.UnitData {
	return &fungibleTokenData{
		tokenType: util.BytesToUint256(attr.TypeID),
		value:     attr.Value,
		t:         currentBlockNr,
		backlink:  txHash,
	}
}

func (n *nonFungibleTokenTypeData) AddToHasher(hasher hash.Hash) {
	hasher.Write([]byte(n.symbol))
	hasher.Write([]byte(n.name))
	hasher.Write([]byte(n.icon.Type))
	hasher.Write(n.icon.Data)
	hasher.Write(util.Uint256ToBytes(n.parentTypeId))
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
	nftTypeID := n.nftType.Bytes32()
	hasher.Write(nftTypeID[:])
	hasher.Write([]byte(n.name))
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
	hasher.Write([]byte(f.name))
	hasher.Write([]byte(f.icon.Type))
	hasher.Write(f.icon.Data)
	parentTypeID := f.parentTypeId.Bytes32()
	hasher.Write(parentTypeID[:])
	hasher.Write(util.Uint32ToBytes(f.decimalPlaces))
	hasher.Write(f.subTypeCreationPredicate)
	hasher.Write(f.tokenCreationPredicate)
	hasher.Write(f.invariantPredicate)
}

func (f *fungibleTokenTypeData) Value() rma.SummaryValue {
	return zeroSummaryValue
}

func (f *fungibleTokenData) AddToHasher(hasher hash.Hash) {
	tokenTypeID := f.tokenType.Bytes32()
	hasher.Write(tokenTypeID[:])
	hasher.Write(util.Uint64ToBytes(f.value))
	hasher.Write(util.Uint64ToBytes(f.t))
	hasher.Write(f.backlink)
}

func (f *fungibleTokenData) Value() rma.SummaryValue {
	return zeroSummaryValue
}
