package twb

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
)

type (
	TokenUnitType struct {
		// common
		ID                       TokenTypeID  `json:"id"`
		ParentTypeID             TokenTypeID  `json:"parentTypeId"`
		Symbol                   string       `json:"symbol"`
		Name                     string       `json:"name,omitempty"`
		Icon                     *tokens.Icon `json:"icon,omitempty"`
		SubTypeCreationPredicate Predicate    `json:"subTypeCreationPredicate,omitempty"`
		TokenCreationPredicate   Predicate    `json:"tokenCreationPredicate,omitempty"`
		InvariantPredicate       Predicate    `json:"invariantPredicate,omitempty"`
		// fungible only
		DecimalPlaces uint32 `json:"decimalPlaces,omitempty"`
		// nft only
		NftDataUpdatePredicate Predicate `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind   Kind   `json:"kind"`
		TxHash TxHash `json:"txHash"`
	}

	TokenUnit struct {
		// common
		ID     TokenID     `json:"id"`
		Symbol string      `json:"symbol"`
		TypeID TokenTypeID `json:"typeId"`
		Owner  Predicate   `json:"owner"`
		// fungible only
		Amount   uint64 `json:"amount,omitempty,string"`
		Decimals uint32 `json:"decimals,omitempty"`
		Burned   bool   `json:"burned,omitempty"`
		// nft only
		NftName                string    `json:"nftName,omitempty"`
		NftURI                 string    `json:"nftUri,omitempty"`
		NftData                []byte    `json:"nftData,omitempty"`
		NftDataUpdatePredicate Predicate `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind   Kind   `json:"kind"`
		TxHash TxHash `json:"txHash"`
	}

	TokenID     UnitID
	TokenTypeID UnitID
	TxHash      []byte
	UnitID      []byte
	Kind        byte

	Proof struct {
		BlockNumber uint64                `json:"blockNumber,string"`
		Tx          *txsystem.Transaction `json:"tx"`
		Proof       *block.BlockProof     `json:"proof"`
	}

	Predicate []byte
	PubKey    []byte
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
