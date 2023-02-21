package twb

import (
	"math"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
)

func Test_storage(t *testing.T) {
	t.Parallel()

	// testing things in one bucket only ie can (re)use the same db
	db := initTestStorage(t)

	t.Run("block number", func(t *testing.T) {
		testBlockNumber(t, db)
	})

	t.Run("token type", func(t *testing.T) {
		testTokenType(t, db)
	})

	t.Run("token type creator", func(t *testing.T) {
		testTokenTypeCreator(t, db)
	})

	t.Run("save token", func(t *testing.T) {
		testSaveToken(t, db)
	})
}

func testTokenTypeCreator(t *testing.T, db *storage) {
	typeID := []byte{0x01}
	creatorKey := PubKey{0x02}

	err := db.SaveTokenTypeCreator(nil, Fungible, creatorKey)
	require.EqualError(t, err, `key required`)

	err = db.SaveTokenTypeCreator(typeID, Fungible, nil)
	require.EqualError(t, err, `bucket type-creator/ not found`)

	require.NoError(t, db.SaveTokenTypeCreator(typeID, Fungible, creatorKey))
}

func testTokenType(t *testing.T, db *storage) {
	proof := &Proof{BlockNumber: 1}
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

	// ID (used as db key) must be assigned
	err := db.SaveTokenType(&TokenUnitType{}, proof)
	require.EqualError(t, err, `failed to save token type data: key required`)

	// TxHash must be assigned
	err = db.SaveTokenType(&TokenUnitType{ID: test.RandomBytes(32)}, proof)
	require.EqualError(t, err, `failed to store unit block proof: key required`)

	// empty db, shouldn't find anything
	typeFromDB, err := db.GetTokenType(typeUnit.ID)
	require.ErrorIs(t, err, errRecordNotFound)
	require.Nil(t, typeFromDB)

	// save a record...
	require.NoError(t, db.SaveTokenType(typeUnit, proof))
	//...and now should find it
	typeFromDB, err = db.GetTokenType(typeUnit.ID)
	require.NoError(t, err)
	require.Equal(t, typeUnit, typeFromDB)
}

func testSaveToken(t *testing.T, db *storage) {
	tokenFromDB, err := db.GetToken(nil)
	require.ErrorIs(t, err, errRecordNotFound)
	require.EqualError(t, err, `failed to read token data token-unit[]: not found`)
	require.Nil(t, tokenFromDB)

	tokenFromDB, err = db.GetToken(test.RandomBytes(32))
	require.ErrorIs(t, err, errRecordNotFound)
	require.Nil(t, tokenFromDB)

	owner := script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32))
	token := randomToken(owner, Fungible)
	proof := &Proof{BlockNumber: 1}

	require.NoError(t, db.SaveToken(token, proof))

	tokenFromDB, err = db.GetToken(token.ID)
	require.NoError(t, err)
	require.Equal(t, token, tokenFromDB)

	// change ownership
	owner2 := script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32))
	token.Owner = owner2
	err = db.SaveToken(token, proof)
	require.NoError(t, err)

	tokenFromDB, err = db.GetToken(token.ID)
	require.NoError(t, err)
	require.Equal(t, token, tokenFromDB)
}

func testBlockNumber(t *testing.T, db *storage) {
	// new empty db, block number should be initaialized to zero
	bn, err := db.GetBlockNumber()
	require.NoError(t, err)
	require.Zero(t, bn)

	getSetBlockNumber := func(value uint64) {
		t.Helper()
		if err := db.SetBlockNumber(value); err != nil {
			t.Fatalf("failed to set block number to %d: %v", value, err)
		}

		bn, err := db.GetBlockNumber()
		if err != nil {
			t.Fatalf("failed to read back block number %d: %v", value, err)
		}
		if bn != value {
			t.Fatalf("expected %d got %d", value, bn)
		}
	}

	getSetBlockNumber(1)
	getSetBlockNumber(0)
	getSetBlockNumber(math.MaxUint32)
	getSetBlockNumber(math.MaxUint64 - 1)
	getSetBlockNumber(math.MaxUint64)

	for i := 0; i < 100; i++ {
		getSetBlockNumber(rand.Uint64())
	}
}

func Test_storage_QueryTokenType(t *testing.T) {
	t.Parallel()

	proof := &Proof{BlockNumber: 1}
	ctorA := test.RandomBytes(32)

	db := initTestStorage(t)

	// empty db, expect nothing to be found
	data, next, err := db.QueryTokenType(Any, nil, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Nil(t, data)

	typ1 := randomTokenType(Fungible)
	require.NoError(t, db.SaveTokenType(typ1, proof))
	// creator relation is not saved so querying by creator should return nothing
	data, next, err = db.QueryTokenType(Any, ctorA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Nil(t, data)
	// but querying without creator should return the type
	data, next, err = db.QueryTokenType(Any, nil, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnitType{typ1})
	// save the creator relation...
	require.NoError(t, db.SaveTokenTypeCreator(typ1.ID, typ1.Kind, ctorA))
	//...and now should succeed quering by creator
	data, next, err = db.QueryTokenType(Any, ctorA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnitType{typ1})

	// add second type
	typ2 := randomTokenType(NonFungible)
	require.NoError(t, db.SaveTokenType(typ2, proof))
	// query one-by-one
	data, next, err = db.QueryTokenType(Any, nil, nil, 1)
	require.NoError(t, err)
	require.NotNil(t, next)
	require.Len(t, data, 1)
	list := append([]*TokenUnitType{}, data[0])
	data, next, err = db.QueryTokenType(Any, nil, next, 1)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Len(t, data, 1)
	list = append(list, data[0])
	require.ElementsMatch(t, list, []*TokenUnitType{typ1, typ2})
	// add creator relation for typ2 and query by owner one itea at the time
	require.NoError(t, db.SaveTokenTypeCreator(typ2.ID, typ2.Kind, ctorA))
	data, next, err = db.QueryTokenType(Any, ctorA, nil, 1)
	require.NoError(t, err)
	require.NotNil(t, next)
	require.Len(t, data, 1)
	list = append([]*TokenUnitType{}, data[0])
	data, next, err = db.QueryTokenType(Any, ctorA, next, 1)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Len(t, data, 1)
	list = append(list, data[0])
	require.ElementsMatch(t, list, []*TokenUnitType{typ1, typ2})

	// query by kind
	data, next, err = db.QueryTokenType(typ1.Kind, nil, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnitType{typ1})

	// query by kind and creator
	data, next, err = db.QueryTokenType(typ2.Kind, ctorA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnitType{typ2})
}

func Test_storage_QueryTokens(t *testing.T) {
	t.Parallel()

	db := initTestStorage(t)

	proof := &Proof{BlockNumber: 1}
	ownerA := script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32))
	ownerB := script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32))

	// empty db, expect nothing to be found
	data, next, err := db.QueryTokens(Any, ownerA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Nil(t, data)
	data, next, err = db.QueryTokens(Any, ownerB, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Nil(t, data)

	// store token for ownerA
	tok1 := randomToken(ownerA, Fungible)
	require.NoError(t, db.SaveToken(tok1, proof))

	data, next, err = db.QueryTokens(Any, ownerA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnit{tok1})
	// no NTF-s
	data, next, err = db.QueryTokens(NonFungible, ownerA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Nil(t, data)
	// but Fungible is there
	data, next, err = db.QueryTokens(Fungible, ownerA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnit{tok1})

	// add second token for ownerA
	tok2 := randomToken(ownerA, NonFungible)
	require.NoError(t, db.SaveToken(tok2, proof))
	// asking for all kinds of tokens, should get both with batch size 10
	data, next, err = db.QueryTokens(Any, ownerA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnit{tok1, tok2})
	// asking for all kinds of tokens with batch size 1 - should get indicator for next
	data, next, err = db.QueryTokens(Any, ownerA, nil, 1)
	require.NoError(t, err)
	require.NotNil(t, next)
	require.Len(t, data, 1)
	list := append([]*TokenUnit{}, data[0])
	//...and now ask for the next token
	data, next, err = db.QueryTokens(Any, ownerA, next, 1)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Len(t, data, 1)
	list = append(list, data[0])
	require.ElementsMatch(t, list, []*TokenUnit{tok1, tok2})
	// NTF-s only
	data, next, err = db.QueryTokens(NonFungible, ownerA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnit{tok2})
	// Fungible only
	data, next, err = db.QueryTokens(Fungible, ownerA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnit{tok1})

	// ownerB should still have no data
	data, next, err = db.QueryTokens(Any, ownerB, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Nil(t, data)
	// transfer tok1 to ownerB
	tok1.Owner = ownerB
	require.NoError(t, db.SaveToken(tok1, proof))
	// quering for ownerA should now have only tok2
	data, next, err = db.QueryTokens(Any, ownerA, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnit{tok2})
	// but ownerB should have tok1
	data, next, err = db.QueryTokens(Any, ownerB, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.ElementsMatch(t, data, []*TokenUnit{tok1})

	// owner is required, when nil empty resultset is returned
	data, next, err = db.QueryTokens(Any, nil, nil, 10)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Nil(t, data)
}

func randomTokenType(kind Kind) *TokenUnitType {
	return &TokenUnitType{
		ID:                       test.RandomBytes(32),
		ParentTypeID:             test.RandomBytes(32),
		Symbol:                   "AB",
		SubTypeCreationPredicate: test.RandomBytes(32),
		TokenCreationPredicate:   test.RandomBytes(32),
		InvariantPredicate:       test.RandomBytes(32),
		DecimalPlaces:            8,
		NftDataUpdatePredicate:   test.RandomBytes(32),
		Kind:                     kind,
		TxHash:                   test.RandomBytes(32),
	}
}

func randomToken(owner Predicate, kind Kind) *TokenUnit {
	return &TokenUnit{
		ID:                     test.RandomBytes(32),
		Symbol:                 "AB",
		TypeID:                 test.RandomBytes(32),
		Owner:                  owner,
		Amount:                 100,
		Decimals:               8,
		NftURI:                 "https://alphabill.org",
		NftData:                test.RandomBytes(32),
		NftDataUpdatePredicate: test.RandomBytes(32),
		Kind:                   kind,
		TxHash:                 test.RandomBytes(32),
	}
}

func initTestStorage(t *testing.T) *storage {
	t.Helper()
	store, err := newBoltStore(filepath.Join(t.TempDir(), "tokens.db"))
	require.NoError(t, err)
	require.NotNil(t, store)

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("%s: store.Close returned error: %v", t.Name(), err)
		}
	})

	return store
}
