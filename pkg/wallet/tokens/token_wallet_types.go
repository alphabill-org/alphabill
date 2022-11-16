package tokens

import (
	"bytes"
	"strings"
)

type (
	TokenUnitType struct {
		Id            TokenTypeId `json:"id"`
		ParentTypeId  TokenTypeId `json:"typeId"`
		Kind          TokenKind   `json:"kind"`
		Symbol        string      `json:"symbol"`
		DecimalPlaces uint32      `json:"decimalPlaces"`
	}

	TokenUnit struct {
		Id       TokenId     `json:"id"`
		Kind     TokenKind   `json:"kind"`
		Symbol   string      `json:"symbol"`
		TypeId   TokenTypeId `json:"typeId"`
		Amount   uint64      `json:"amount"`        // fungible only
		URI      string      `json:"uri,omitempty"` // nft only
		Backlink []byte      `json:"backlink"`
	}

	TokenKind uint

	TokenId     []byte
	TokenTypeId []byte

	TokenWithOwner struct {
		Token *TokenUnit
		Owner PublicKey
	}

	TokenTypeInfo interface {
		GetSymbol() string
		GetTypeId() TokenTypeId
	}

	PublicKey []byte
)

const (
	txTimeoutBlockCount               = 100
	AllAccounts                   int = -1
	alwaysTrueTokensAccountNumber     = 0

	Any TokenKind = 1 << iota
	TokenType
	Token
	Fungible
	NonFungible

	FungibleTokenType    = TokenType | Fungible
	NonFungibleTokenType = TokenType | NonFungible
	FungibleToken        = Token | Fungible
	NonFungibleToken     = Token | NonFungible
)

func (t *TokenUnit) IsFungible() bool {
	return t.Kind&FungibleToken == FungibleToken
}

func (k *TokenKind) String() string {
	if *k&Any != 0 {
		return "[any]"
	}
	res := make([]string, 0)
	if *k&TokenType != 0 {
		res = append(res, "type")
	} else {
		res = append(res, "token")
	}
	if *k&Fungible != 0 {
		res = append(res, "fungible")
	} else {
		res = append(res, "non-fungible")
	}
	return "[" + strings.Join(res, ",") + "]"
}

func (t TokenTypeId) equal(to TokenTypeId) bool {
	return bytes.Equal(t, to)
}

func (tp *TokenUnitType) GetSymbol() string {
	return tp.Symbol
}

func (tp *TokenUnitType) GetTypeId() TokenTypeId {
	return tp.Id
}

func (t *TokenUnit) GetSymbol() string {
	return t.Symbol
}

func (t *TokenUnit) GetTypeId() TokenTypeId {
	return t.TypeId
}

func (id TokenId) String() string {
	return string(id)
}
