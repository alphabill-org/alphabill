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

type noneFungibleTokenData struct {
	nftType             *uint256.Int
	uri                 string // uri is the optional URI of an external resource associated with the token
	data                []byte // data is the optional data associated with the token.
	dataUpdatePredicate []byte // the data update predicate;
	t                   uint64 // the round number of the last transaction with this token;
	backlink            []byte // the hash of the last transaction order for this token
}

func newMintNonFungibleTokenData(tx *mintNonFungibleTokenWrapper, hasher crypto.Hash) rma.UnitData {
	attr := tx.attributes
	return &noneFungibleTokenData{
		nftType:             tx.NFTTypeID(),
		uri:                 attr.Uri,
		data:                attr.Data,
		dataUpdatePredicate: attr.DataUpdatePredicate,
		t:                   0,                           // we don't have previous tx
		backlink:            make([]byte, hasher.Size()), // in case of new NFT token the backlink is zero hash
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

func (n *noneFungibleTokenData) AddToHasher(hasher hash.Hash) {
	hasher.Write(n.nftType.Bytes())
	hasher.Write([]byte(n.uri))
	hasher.Write(n.data)
	hasher.Write(n.dataUpdatePredicate)
	hasher.Write(util.Uint64ToBytes(n.t))
	hasher.Write(n.backlink)
}

func (n *noneFungibleTokenData) Value() rma.SummaryValue {
	return zeroSummaryValue
}
