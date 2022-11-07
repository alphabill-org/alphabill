package tokens

import (
	"bytes"
	"strings"
)

type (
	tokenType struct {
		Id            TokenTypeId `json:"id"`
		ParentTypeId  TokenTypeId `json:"typeId"`
		Kind          TokenKind   `json:"kind"`
		Symbol        string      `json:"symbol"`
		DecimalPlaces uint32      `json:"decimalPlaces"`
	}

	token struct {
		Id       TokenId     `json:"id"`
		Kind     TokenKind   `json:"kind"`
		Symbol   string      `json:"symbol"`
		TypeId   TokenTypeId `json:"typeId"`
		Amount   uint64      `json:"amount"` // fungible only
		Uri      string      `json:"uri"`    // nft only
		Backlink []byte      `json:"backlink"`
	}

	TokenKind uint

	TokenId     []byte
	TokenTypeId []byte
)

const (
	Any TokenKind = 1 << iota
	TokenType
	Token
	Fungible
	NonFungible
	FungibleToken    = Token | Fungible
	NonFungibleToken = Token | NonFungible
)

func (t *token) isFungible() bool {
	return t.Kind&FungibleToken == FungibleToken
}

func (k *TokenKind) pretty() string {
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

type TokenTypeInfo interface {
	GetSymbol() string
	GetTypeId() TokenTypeId
}

func (tp *tokenType) GetSymbol() string {
	return tp.Symbol
}

func (tp *tokenType) GetTypeId() TokenTypeId {
	return tp.Id
}

func (t *token) GetSymbol() string {
	return t.Symbol
}

func (t *token) GetTypeId() TokenTypeId {
	return t.TypeId
}

func (id TokenId) string() string {
	return string(id)
}
