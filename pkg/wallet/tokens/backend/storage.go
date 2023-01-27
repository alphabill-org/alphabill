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

		SaveTokenType(data *TokenUnitType) error
		GetTokenType(id TokenTypeID) (*TokenUnitType, error)

		SaveToken(data *TokenUnit) error
		GetToken(id TokenID) (*TokenUnit, error)
	}
)

type (
	TokenUnitType struct {
		// common
		ID                       TokenTypeID `json:"id"`
		ParentTypeID             TokenTypeID `json:"typeId"`
		Symbol                   string      `json:"symbol"`
		SubTypeCreationPredicate []byte      `json:"subTypeCreationPredicate,omitempty"`
		TokenCreationPredicate   []byte      `json:"tokenCreationPredicate,omitempty"`
		InvariantPredicate       []byte      `json:"invariantPredicate,omitempty"`
		// fungible only
		DecimalPlaces uint32 `json:"decimalPlaces,omitempty"`
		// nft only
		NftDataUpdatePredicate []byte `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind  Kind   `json:"kind"`
		Proof *Proof `json:"proof"`
	}

	TokenUnit struct {
		// Backlink []byte  `json:"backlink"` // TODO: Proof.TxHash can be used instead
		// common
		ID     TokenID     `json:"id"`
		Symbol string      `json:"symbol"`
		TypeID TokenTypeID `json:"typeId"`
		// fungible only
		Amount   uint64 `json:"amount"`
		Decimals uint32 `json:"decimals,omitempty"`
		// nft only
		NftURI                 string `json:"nftUri,omitempty"`
		NftData                []byte `json:"nftData,omitempty"`
		NftDataUpdatePredicate []byte `json:"nftDataUpdatePredicate,omitempty"`
		// meta
		Kind  Kind   `json:"kind"`
		Proof *Proof `json:"proof"`
	}

	TokenID     []byte
	TokenTypeID []byte
	Kind        byte

	Proof struct {
		BlockNumber uint64                `json:"blockNumber"`
		Tx          *txsystem.Transaction `json:"tx"`
		TxHash      []byte                `json:"txHash"`
		Proof       *block.BlockProof     `json:"proof"`
	}
)

const (
	Any Kind = 1 << iota
	Fungible
	NonFungible
)
