package twb

import (
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

type (
	Storage interface {
		Close() error
		GetBlockNumber() (uint64, error)
		SetBlockNumber(blockNumber uint64) error

		SaveTokenType(data *TokenUnitType, proof *Proof) error
		GetTokenType(id TokenTypeID) (*TokenUnitType, error)

		SaveToken(data *TokenUnit, proof *Proof) error
		GetToken(id TokenID) (*TokenUnit, error)
	}
)

type (
	TokenUnitType struct {
		// common
		ID                       TokenTypeID `json:"id"`
		ParentTypeID             TokenTypeID `json:"typeId"`
		Symbol                   string      `json:"symbol"`
		SubTypeCreationPredicate Predicate   `json:"subTypeCreationPredicate,omitempty"`
		TokenCreationPredicate   Predicate   `json:"tokenCreationPredicate,omitempty"`
		InvariantPredicate       Predicate   `json:"invariantPredicate,omitempty"`
		// fungible only
		DecimalPlaces uint32 `json:"decimalPlaces,omitempty"`
		// nft only
		NftDataUpdatePredicate Predicate `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind   Kind   `json:"kind"`
		TxHash []byte `json:"txHash"`
	}

	TokenUnit struct {
		// common
		ID     TokenID     `json:"id"`
		Symbol string      `json:"symbol"`
		TypeID TokenTypeID `json:"typeId"`
		Owner  Predicate   `json:"owner"`
		// fungible only
		Amount   uint64 `json:"amount"`
		Decimals uint32 `json:"decimals,omitempty"`
		// nft only
		NftURI                 string    `json:"nftUri,omitempty"`
		NftData                Predicate `json:"nftData,omitempty"`
		NftDataUpdatePredicate Predicate `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind   Kind   `json:"kind"`
		TxHash []byte `json:"txHash"`
	}

	TokenID     []byte
	TokenTypeID []byte
	Kind        byte

	Proof struct {
		BlockNumber uint64                `json:"blockNumber"`
		Tx          *txsystem.Transaction `json:"tx"`
		Proof       *block.BlockProof     `json:"proof"`
	}

	Predicate []byte
)

const (
	Any Kind = 1 << iota
	Fungible
	NonFungible
)
