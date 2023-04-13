package twb

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/pkg/wallet"
)

type (
	TokenUnitType struct {
		// common
		ID                       TokenTypeID      `json:"id"`
		ParentTypeID             TokenTypeID      `json:"parentTypeId"`
		Symbol                   string           `json:"symbol"`
		SubTypeCreationPredicate wallet.Predicate `json:"subTypeCreationPredicate,omitempty"`
		TokenCreationPredicate   wallet.Predicate `json:"tokenCreationPredicate,omitempty"`
		InvariantPredicate       wallet.Predicate `json:"invariantPredicate,omitempty"`
		// fungible only
		DecimalPlaces uint32 `json:"decimalPlaces,omitempty"`
		// nft only
		NftDataUpdatePredicate wallet.Predicate `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind   Kind          `json:"kind"`
		TxHash wallet.TxHash `json:"txHash"`
	}

	TokenUnit struct {
		// common
		ID     TokenID          `json:"id"`
		Symbol string           `json:"symbol"`
		TypeID TokenTypeID      `json:"typeId"`
		Owner  wallet.Predicate `json:"owner"`
		// fungible only
		Amount   uint64 `json:"amount,omitempty,string"`
		Decimals uint32 `json:"decimals,omitempty"`
		// nft only
		NftURI                 string           `json:"nftUri,omitempty"`
		NftData                []byte           `json:"nftData,omitempty"`
		NftDataUpdatePredicate wallet.Predicate `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind   Kind          `json:"kind"`
		TxHash wallet.TxHash `json:"txHash"`
	}

	TokenID     wallet.UnitID
	TokenTypeID wallet.UnitID

	Kind byte
)

const (
	Any Kind = 1 << iota
	Fungible
	NonFungible
)

var (
	NoParent = TokenTypeID{0x00}
)

func (t TokenTypeID) Equal(to TokenTypeID) bool {
	return bytes.Equal(t, to)
}

func (kind Kind) String() string {
	switch kind {
	case Any:
		return "all"
	case Fungible:
		return "fungible"
	case NonFungible:
		return "nft"
	}
	return "unknown"
}

func strToTokenKind(s string) (Kind, error) {
	switch s {
	case "all", "":
		return Any, nil
	case "fungible":
		return Fungible, nil
	case "nft":
		return NonFungible, nil
	}
	return Any, fmt.Errorf("%q is not valid token kind", s)
}
