package tokens

import (
	"bytes"
	"fmt"
	"hash"
	"strings"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

type NonFungibleTokenTypeData struct {
	_                        struct{} `cbor:",toarray"`
	Symbol                   string
	Name                     string
	Icon                     *Icon
	ParentTypeId             types.UnitID // identifies the parent type that this type derives from; 0 indicates there is no parent type;
	SubTypeCreationPredicate []byte       // the predicate clause that controls defining new sub-types of this type;
	TokenCreationPredicate   []byte       // the predicate clause that controls creating new tokens of this type
	InvariantPredicate       []byte       // the invariant predicate clause that all tokens of this type (and of sub-types of this type) inherit into their bearer predicates;
	DataUpdatePredicate      []byte       // the clause that all tokens of this type (and of sub-types of this type) inherit into their data update predicates
}

type FungibleTokenTypeData struct {
	_                        struct{} `cbor:",toarray"`
	Symbol                   string
	Name                     string
	Icon                     *Icon
	ParentTypeId             types.UnitID // identifies the parent type that this type derives from; 0 indicates there is no parent type;
	DecimalPlaces            uint32       // is the number of decimal places to display for values of tokens of this type;
	SubTypeCreationPredicate []byte       // the predicate clause that controls defining new sub-types of this type;
	TokenCreationPredicate   []byte       // the predicate clause that controls creating new tokens of this type
	InvariantPredicate       []byte       // the invariant predicate clause that all tokens of this type (and of sub-types of this type) inherit into their bearer predicates;
}

type NonFungibleTokenData struct {
	_                   struct{} `cbor:",toarray"`
	NftType             types.UnitID
	Name                string // the optional long name of the token
	URI                 string // uri is the optional URI of an external resource associated with the token
	Data                []byte // data is the optional data associated with the token.
	DataUpdatePredicate []byte // the data update predicate;
	T                   uint64 // the round number of the last transaction with this token;
	Backlink            []byte // the hash of the last transaction order for this token
	Locked              uint64 // locked status of the bill, non-zero value means locked
}

type FungibleTokenData struct {
	_         struct{}     `cbor:",toarray"`
	TokenType types.UnitID // the type of the token;
	Value     uint64       // the value of the token;
	T         uint64       // the round number of the last transaction with this token;
	backlink  []byte       // the hash of the last transaction order for this token
	locked    uint64       // locked status of the bill, non-zero value means locked
}

func newFungibleTokenTypeData(attr *CreateFungibleTokenTypeAttributes) state.UnitData {
	return &FungibleTokenTypeData{
		Symbol:                   attr.Symbol,
		Name:                     attr.Name,
		Icon:                     attr.Icon,
		ParentTypeId:             attr.ParentTypeID,
		DecimalPlaces:            attr.DecimalPlaces,
		SubTypeCreationPredicate: attr.SubTypeCreationPredicate,
		TokenCreationPredicate:   attr.TokenCreationPredicate,
		InvariantPredicate:       attr.InvariantPredicate,
	}
}

func newNonFungibleTokenTypeData(attr *CreateNonFungibleTokenTypeAttributes) state.UnitData {
	return &NonFungibleTokenTypeData{
		Symbol:                   attr.Symbol,
		Name:                     attr.Name,
		Icon:                     attr.Icon,
		ParentTypeId:             attr.ParentTypeID,
		SubTypeCreationPredicate: attr.SubTypeCreationPredicate,
		TokenCreationPredicate:   attr.TokenCreationPredicate,
		InvariantPredicate:       attr.InvariantPredicate,
		DataUpdatePredicate:      attr.DataUpdatePredicate,
	}
}

func newNonFungibleTokenData(attr *MintNonFungibleTokenAttributes, txHash []byte, currentBlockNr uint64) state.UnitData {
	return &NonFungibleTokenData{
		NftType:             attr.NFTTypeID,
		Name:                attr.Name,
		URI:                 attr.URI,
		Data:                attr.Data,
		DataUpdatePredicate: attr.DataUpdatePredicate,
		T:                   currentBlockNr,
		Backlink:            txHash,
	}
}

func newFungibleTokenData(attr *MintFungibleTokenAttributes, txHash []byte, currentBlockNr uint64) state.UnitData {
	return &FungibleTokenData{
		TokenType: attr.TypeID,
		Value:     attr.Value,
		T:         currentBlockNr,
		backlink:  txHash,
	}
}

func (n *NonFungibleTokenTypeData) Write(hasher hash.Hash) error {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(n)
	if err != nil {
		return fmt.Errorf("nft type serialization error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (n *NonFungibleTokenTypeData) SummaryValueInput() uint64 {
	return 0
}

func (n *NonFungibleTokenTypeData) Copy() state.UnitData {
	if n == nil {
		return nil
	}
	return &NonFungibleTokenTypeData{
		Symbol:                   strings.Clone(n.Symbol),
		Name:                     strings.Clone(n.Name),
		Icon:                     n.Icon.Copy(),
		ParentTypeId:             bytes.Clone(n.ParentTypeId),
		SubTypeCreationPredicate: bytes.Clone(n.SubTypeCreationPredicate),
		TokenCreationPredicate:   bytes.Clone(n.TokenCreationPredicate),
		InvariantPredicate:       bytes.Clone(n.InvariantPredicate),
		DataUpdatePredicate:      bytes.Clone(n.DataUpdatePredicate),
	}
}

func (n *NonFungibleTokenData) Write(hasher hash.Hash) error {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(n)
	if err != nil {
		return fmt.Errorf("ft data serialization error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (n *NonFungibleTokenData) SummaryValueInput() uint64 {
	return 0
}

func (n *NonFungibleTokenData) Copy() state.UnitData {
	if n == nil {
		return nil
	}
	return &NonFungibleTokenData{
		NftType:             bytes.Clone(n.NftType),
		Name:                strings.Clone(n.Name),
		URI:                 strings.Clone(n.URI),
		Data:                bytes.Clone(n.Data),
		DataUpdatePredicate: bytes.Clone(n.DataUpdatePredicate),
		T:                   n.T,
		Backlink:            bytes.Clone(n.Backlink),
		Locked:              n.Locked,
	}
}

func (n *NonFungibleTokenData) GetBacklink() []byte {
	return n.Backlink
}

func (n *NonFungibleTokenData) IsLocked() uint64 {
	return n.Locked
}

func (f *FungibleTokenTypeData) Write(hasher hash.Hash) error {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(f)
	if err != nil {
		return fmt.Errorf("ft type serialization error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (f *FungibleTokenTypeData) SummaryValueInput() uint64 {
	return 0
}

func (f *FungibleTokenTypeData) Copy() state.UnitData {
	if f == nil {
		return nil
	}
	return &FungibleTokenTypeData{
		Symbol:                   strings.Clone(f.Symbol),
		Name:                     strings.Clone(f.Name),
		Icon:                     f.Icon.Copy(),
		ParentTypeId:             bytes.Clone(f.ParentTypeId),
		DecimalPlaces:            f.DecimalPlaces,
		SubTypeCreationPredicate: bytes.Clone(f.SubTypeCreationPredicate),
		TokenCreationPredicate:   bytes.Clone(f.TokenCreationPredicate),
		InvariantPredicate:       bytes.Clone(f.InvariantPredicate),
	}
}

func (f *FungibleTokenData) Write(hasher hash.Hash) error {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(f)
	if err != nil {
		return fmt.Errorf("ft data serialization error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (f *FungibleTokenData) SummaryValueInput() uint64 {
	return 0
}

func (f *FungibleTokenData) Copy() state.UnitData {
	if f == nil {
		return nil
	}
	return &FungibleTokenData{
		TokenType: bytes.Clone(f.TokenType),
		Value:     f.Value,
		T:         f.T,
		backlink:  bytes.Clone(f.backlink),
		locked:    f.locked,
	}
}

func (f *FungibleTokenData) GetBacklink() []byte {
	return f.backlink
}

func (f *FungibleTokenData) IsLocked() uint64 {
	return f.locked
}
