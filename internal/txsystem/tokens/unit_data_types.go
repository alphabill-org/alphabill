package tokens

import (
	"bytes"
	"hash"
	"strings"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

type nonFungibleTokenTypeData struct {
	symbol                   string
	name                     string
	icon                     *Icon
	parentTypeId             types.UnitID // identifies the parent type that this type derives from; 0 indicates there is no parent type;
	subTypeCreationPredicate []byte       // the predicate clause that controls defining new sub-types of this type;
	tokenCreationPredicate   []byte       // the predicate clause that controls creating new tokens of this type
	invariantPredicate       []byte       // the invariant predicate clause that all tokens of this type (and of sub-types of this type) inherit into their bearer predicates;
	dataUpdatePredicate      []byte       // the clause that all tokens of this type (and of sub-types of this type) inherit into their data update predicates
}

type fungibleTokenTypeData struct {
	symbol                   string
	name                     string
	icon                     *Icon
	parentTypeId             types.UnitID // identifies the parent type that this type derives from; 0 indicates there is no parent type;
	decimalPlaces            uint32       // is the number of decimal places to display for values of tokens of this type;
	subTypeCreationPredicate []byte       // the predicate clause that controls defining new sub-types of this type;
	tokenCreationPredicate   []byte       // the predicate clause that controls creating new tokens of this type
	invariantPredicate       []byte       // the invariant predicate clause that all tokens of this type (and of sub-types of this type) inherit into their bearer predicates;
}

type nonFungibleTokenData struct {
	nftType             types.UnitID
	name                string // the optional long name of the token
	uri                 string // uri is the optional URI of an external resource associated with the token
	data                []byte // data is the optional data associated with the token.
	dataUpdatePredicate []byte // the data update predicate;
	t                   uint64 // the round number of the last transaction with this token;
	backlink            []byte // the hash of the last transaction order for this token
	locked              uint64 // locked status of the bill, non-zero value means locked
}

type fungibleTokenData struct {
	tokenType types.UnitID // the type of the token;
	value     uint64       // the value of the token;
	t         uint64       // the round number of the last transaction with this token;
	backlink  []byte       // the hash of the last transaction order for this token
	locked    uint64       // locked status of the bill, non-zero value means locked
}

func newFungibleTokenTypeData(attr *CreateFungibleTokenTypeAttributes) state.UnitData {
	return &fungibleTokenTypeData{
		symbol:                   attr.Symbol,
		name:                     attr.Name,
		icon:                     attr.Icon,
		parentTypeId:             attr.ParentTypeID,
		decimalPlaces:            attr.DecimalPlaces,
		subTypeCreationPredicate: attr.SubTypeCreationPredicate,
		tokenCreationPredicate:   attr.TokenCreationPredicate,
		invariantPredicate:       attr.InvariantPredicate,
	}
}

func newNonFungibleTokenTypeData(attr *CreateNonFungibleTokenTypeAttributes) state.UnitData {
	return &nonFungibleTokenTypeData{
		symbol:                   attr.Symbol,
		name:                     attr.Name,
		icon:                     attr.Icon,
		parentTypeId:             attr.ParentTypeID,
		subTypeCreationPredicate: attr.SubTypeCreationPredicate,
		tokenCreationPredicate:   attr.TokenCreationPredicate,
		invariantPredicate:       attr.InvariantPredicate,
		dataUpdatePredicate:      attr.DataUpdatePredicate,
	}
}

func newNonFungibleTokenData(attr *MintNonFungibleTokenAttributes, txHash []byte, currentBlockNr uint64) state.UnitData {
	return &nonFungibleTokenData{
		nftType:             attr.NFTTypeID,
		name:                attr.Name,
		uri:                 attr.URI,
		data:                attr.Data,
		dataUpdatePredicate: attr.DataUpdatePredicate,
		t:                   currentBlockNr,
		backlink:            txHash,
	}
}

func newFungibleTokenData(attr *MintFungibleTokenAttributes, txHash []byte, currentBlockNr uint64) state.UnitData {
	return &fungibleTokenData{
		tokenType: attr.TypeID,
		value:     attr.Value,
		t:         currentBlockNr,
		backlink:  txHash,
	}
}

func (n *nonFungibleTokenTypeData) Write(hasher hash.Hash) {
	hasher.Write([]byte(n.symbol))
	hasher.Write([]byte(n.name))
	n.icon.AddToHasher(hasher)
	hasher.Write(n.parentTypeId)
	hasher.Write(n.subTypeCreationPredicate)
	hasher.Write(n.tokenCreationPredicate)
	hasher.Write(n.invariantPredicate)
	hasher.Write(n.dataUpdatePredicate)
}

func (n *nonFungibleTokenTypeData) SummaryValueInput() uint64 {
	return 0
}

func (n *nonFungibleTokenTypeData) Copy() state.UnitData {
	if n == nil {
		return nil
	}
	return &nonFungibleTokenTypeData{
		symbol:                   strings.Clone(n.symbol),
		name:                     strings.Clone(n.name),
		icon:                     n.icon.Copy(),
		parentTypeId:             bytes.Clone(n.parentTypeId),
		subTypeCreationPredicate: bytes.Clone(n.subTypeCreationPredicate),
		tokenCreationPredicate:   bytes.Clone(n.tokenCreationPredicate),
		invariantPredicate:       bytes.Clone(n.invariantPredicate),
		dataUpdatePredicate:      bytes.Clone(n.dataUpdatePredicate),
	}
}

func (n *nonFungibleTokenData) Write(hasher hash.Hash) {
	hasher.Write(n.nftType)
	hasher.Write([]byte(n.name))
	hasher.Write([]byte(n.uri))
	hasher.Write(n.data)
	hasher.Write(n.dataUpdatePredicate)
	hasher.Write(util.Uint64ToBytes(n.t))
	hasher.Write(n.backlink)
	hasher.Write(util.Uint64ToBytes(n.locked))
}

func (n *nonFungibleTokenData) SummaryValueInput() uint64 {
	return 0
}

func (n *nonFungibleTokenData) Copy() state.UnitData {
	if n == nil {
		return nil
	}
	return &nonFungibleTokenData{
		nftType:             bytes.Clone(n.nftType),
		name:                strings.Clone(n.name),
		uri:                 strings.Clone(n.uri),
		data:                bytes.Clone(n.data),
		dataUpdatePredicate: bytes.Clone(n.dataUpdatePredicate),
		t:                   n.t,
		backlink:            bytes.Clone(n.backlink),
		locked:              n.locked,
	}
}

func (n *nonFungibleTokenData) Backlink() []byte {
	return n.backlink
}

func (n *nonFungibleTokenData) Locked() uint64 {
	return n.locked
}

func (f *fungibleTokenTypeData) Write(hasher hash.Hash) {
	hasher.Write([]byte(f.symbol))
	hasher.Write([]byte(f.name))
	f.icon.AddToHasher(hasher)
	hasher.Write(f.parentTypeId)
	hasher.Write(util.Uint32ToBytes(f.decimalPlaces))
	hasher.Write(f.subTypeCreationPredicate)
	hasher.Write(f.tokenCreationPredicate)
	hasher.Write(f.invariantPredicate)
}

func (f *fungibleTokenTypeData) SummaryValueInput() uint64 {
	return 0
}

func (f *fungibleTokenTypeData) Copy() state.UnitData {
	if f == nil {
		return nil
	}
	return &fungibleTokenTypeData{
		symbol:                   strings.Clone(f.symbol),
		name:                     strings.Clone(f.name),
		icon:                     f.icon.Copy(),
		parentTypeId:             bytes.Clone(f.parentTypeId),
		decimalPlaces:            f.decimalPlaces,
		subTypeCreationPredicate: bytes.Clone(f.subTypeCreationPredicate),
		tokenCreationPredicate:   bytes.Clone(f.tokenCreationPredicate),
		invariantPredicate:       bytes.Clone(f.invariantPredicate),
	}
}

func (f *fungibleTokenData) Write(hasher hash.Hash) {
	hasher.Write(f.tokenType)
	hasher.Write(util.Uint64ToBytes(f.value))
	hasher.Write(util.Uint64ToBytes(f.t))
	hasher.Write(f.backlink)
	hasher.Write(util.Uint64ToBytes(f.locked))
}

func (f *fungibleTokenData) SummaryValueInput() uint64 {
	return 0
}

func (f *fungibleTokenData) Copy() state.UnitData {
	if f == nil {
		return nil
	}
	return &fungibleTokenData{
		tokenType: bytes.Clone(f.tokenType),
		value:     f.value,
		t:         f.t,
		backlink:  bytes.Clone(f.backlink),
		locked:    f.locked,
	}
}

func (f *fungibleTokenData) Backlink() []byte {
	return f.backlink
}

func (f *fungibleTokenData) Locked() uint64 {
	return f.locked
}

func (i *Icon) AddToHasher(hasher hash.Hash) {
	if i == nil {
		return
	}
	hasher.Write([]byte(i.Type))
	hasher.Write(i.Data)
}
