package tokens

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenTypes_fungible(t *testing.T) {
	db := openTestDB(t)
	id, err := RandomId()
	require.NoError(t, err)
	typeFun := &TokenUnitType{
		ID:            TokenTypeID(id),
		Kind:          FungibleTokenType,
		Symbol:        "AB",
		DecimalPlaces: 2,
	}
	require.NoError(t, db.Do().AddTokenType(typeFun))
	typeFromDB, err := db.Do().GetTokenType(TokenTypeID(id))
	require.NoError(t, err)
	require.Equal(t, typeFun, typeFromDB)
	types, err := db.Do().GetTokenTypes()
	require.NoError(t, err)
	require.Len(t, types, 1)
	require.Contains(t, types, typeFun)
}

func TestTokenTypes_nft(t *testing.T) {
	db := openTestDB(t)
	id, err := RandomId()
	require.NoError(t, err)
	typeNft := &TokenUnitType{
		ID:     TokenTypeID(id),
		Kind:   NonFungibleTokenType,
		Symbol: "AB",
	}
	require.NoError(t, db.Do().AddTokenType(typeNft))
	typeFromDB, err := db.Do().GetTokenType(TokenTypeID(id))
	require.NoError(t, err)
	require.Equal(t, typeNft, typeFromDB)
	types, err := db.Do().GetTokenTypes()
	require.NoError(t, err)
	require.Len(t, types, 1)
	require.Contains(t, types, typeNft)
}

func TestTokens_fungible(t *testing.T) {
	db := openTestDB(t)
	typeId, err := RandomId()
	require.NoError(t, err)
	id, err := RandomId()
	require.NoError(t, err)
	token := &TokenUnit{
		ID:       id,
		Kind:     FungibleToken,
		TypeID:   TokenTypeID(typeId),
		Amount:   uint64(100),
		Backlink: nil,
	}
	acc := uint64(1)
	require.NoError(t, db.Do().SetToken(acc, token))
	typeFromDB, err := db.Do().GetToken(acc, id)
	require.NoError(t, err)
	require.Equal(t, token, typeFromDB)
	toks, err := db.Do().GetTokens(acc)
	require.NoError(t, err)
	require.Len(t, toks, 1)
	require.Contains(t, toks, token)
	require.NoError(t, db.Do().RemoveToken(acc, token.ID))
	typeFromDB, err = db.Do().GetToken(acc, id)
	require.NoError(t, err)
	require.Nil(t, typeFromDB)
}

func TestTokens_nft(t *testing.T) {
	db := openTestDB(t)
	typeId, err := RandomId()
	require.NoError(t, err)
	id, err := RandomId()
	require.NoError(t, err)
	token := &TokenUnit{
		ID:       id,
		Kind:     NonFungibleToken,
		TypeID:   TokenTypeID(typeId),
		URI:      "https://alphabill.org",
		Backlink: nil,
	}
	acc := uint64(1)
	require.NoError(t, db.Do().SetToken(acc, token))
	typeFromDB, err := db.Do().GetToken(acc, id)
	require.NoError(t, err)
	require.Equal(t, token, typeFromDB)
	toks, err := db.Do().GetTokens(acc)
	require.NoError(t, err)
	require.Len(t, toks, 1)
	require.Contains(t, toks, token)
	require.NoError(t, db.Do().RemoveToken(acc, token.ID))
	typeFromDB, err = db.Do().GetToken(acc, id)
	require.NoError(t, err)
	require.Nil(t, typeFromDB)
}

func TestTokens_blockNumbers(t *testing.T) {
	db := openTestDB(t)

	blockNr, err := db.Do().GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, uint64(0), blockNr)

	require.NoError(t, db.Do().SetBlockNumber(1))
	blockNr, err = db.Do().GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, uint64(1), blockNr)
}

func openTestDB(t *testing.T) *tokensDb {
	_ = deleteFile(os.TempDir(), tokensFileName)
	db, err := openTokensDb(os.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		db.DeleteDb()
	})
	return db
}
