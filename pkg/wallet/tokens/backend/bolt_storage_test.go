package twb

import (
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestStorage_TokenTypeCreator(t *testing.T) {
	store := initTestStorage(t)
	defer func() { require.NoError(t, store.Close()) }()

	typeID := []byte{0x01}
	creatorKey := PubKey{0x02}
	err := store.SaveTokenTypeCreator(typeID, creatorKey)
	require.NoError(t, err)

	require.NoError(t, store.db.View(
		func(tx *bolt.Tx) error {
			typeFromDB, err := store.getTokenTypeIDsByCreator(tx, creatorKey)
			require.NoError(t, err)
			require.Len(t, typeFromDB, 1)
			require.Equal(t, TokenTypeID(typeID), typeFromDB[0])
			return nil
		}))

	// expect no token type data in return
	types, err := store.GetTokenTypesByCreator(creatorKey)
	require.NoError(t, err)
	require.Len(t, types, 0)

	// add type units
	err = store.SaveTokenType(&TokenUnitType{ID: typeID, TxHash: []byte{0x04}}, &Proof{})
	require.NoError(t, err)
	err = store.SaveTokenType(&TokenUnitType{ID: []byte{0x03}, TxHash: []byte{0x05}}, &Proof{})
	require.NoError(t, err)

	types, err = store.GetTokenTypesByCreator(creatorKey)
	require.NoError(t, err)
	require.Len(t, types, 1)
	require.Equal(t, TokenTypeID(typeID), types[0].ID)
}

func TestStorage_SaveTokenType(t *testing.T) {
	store := initTestStorage(t)
	defer func() { require.NoError(t, store.Close()) }()

	// fill all fields with random stuff
	typeUnit := &TokenUnitType{
		ID:                       test.RandomBytes(32),
		ParentTypeID:             test.RandomBytes(32),
		Symbol:                   "AB",
		SubTypeCreationPredicate: test.RandomBytes(32),
		TokenCreationPredicate:   test.RandomBytes(32),
		InvariantPredicate:       test.RandomBytes(32),
		DecimalPlaces:            8,
		NftDataUpdatePredicate:   test.RandomBytes(32),
		Kind:                     Fungible,
		TxHash:                   test.RandomBytes(32),
	}

	proof := &Proof{BlockNumber: 1}

	err := store.SaveTokenType(typeUnit, proof)
	require.NoError(t, err)

	typeFromDB, err := store.GetTokenType(typeUnit.ID)
	require.NoError(t, err)
	require.Equal(t, typeUnit, typeFromDB)

	// TODO: check proof
}

func TestStorage_SaveToken(t *testing.T) {
	store := initTestStorage(t)
	defer func() { require.NoError(t, store.Close()) }()

	owner := script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32))

	tokens, err := store.GetTokensByOwner(owner)
	require.NoError(t, err)
	require.Len(t, tokens, 0)

	// fill all fields with random stuff
	token := &TokenUnit{
		ID:                     test.RandomBytes(32),
		Symbol:                 "AB",
		TypeID:                 test.RandomBytes(32),
		Owner:                  owner,
		Amount:                 100,
		Decimals:               8,
		NftURI:                 "https://alphabill.org",
		NftData:                test.RandomBytes(32),
		NftDataUpdatePredicate: test.RandomBytes(32),
		Kind:                   Fungible,
		TxHash:                 test.RandomBytes(32),
	}

	proof := &Proof{BlockNumber: 1}

	err = store.SaveToken(token, proof)
	require.NoError(t, err)

	tokenFromDB, err := store.GetToken(token.ID)
	require.NoError(t, err)
	require.Equal(t, token, tokenFromDB)

	tokens, err = store.GetTokensByOwner(owner)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, token, tokens[0])

	// TODO: check proof
}

func TestStorage_BlockNumbers(t *testing.T) {
	store := initTestStorage(t)
	defer func() { require.NoError(t, store.Close()) }()

	require.NoError(t, store.SetBlockNumber(2))

	blockNumbers, err := store.GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, uint64(2), blockNumbers)

	// let's not go back in time
	require.Error(t, store.SetBlockNumber(2))
	require.Error(t, store.SetBlockNumber(1))
}

func initTestStorage(t *testing.T) *storage {
	store, err := newBoltStore(filepath.Join(t.TempDir(), BoltTokenStoreFileName))
	require.NoError(t, err)
	return store
}
