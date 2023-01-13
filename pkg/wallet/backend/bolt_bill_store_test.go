package backend

import (
	"path"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestBillStore_CanBeCreated(t *testing.T) {
	bs, err := createTestBillStore(t)
	require.NoError(t, err)
	require.NotNil(t, bs)
}

func TestBlockStore_GetSetBlockNumber(t *testing.T) {
	bs, _ := createTestBillStore(t)

	// verify initial block number is 0
	blockNumber, err := bs.GetBlockNumber()
	require.NoError(t, err)
	require.EqualValues(t, 0, blockNumber)

	// set block number
	err = bs.SetBlockNumber(1)
	require.NoError(t, err)

	// verify block number
	blockNumber, err = bs.GetBlockNumber()
	require.NoError(t, err)
	require.EqualValues(t, 1, blockNumber)
}

func TestBillStore_GetSetBills(t *testing.T) {
	bs, _ := createTestBillStore(t)
	pubKey, _ := hexutil.Decode("0x000000000000000000000000000000000000000000000000000000000000000000")

	// verify non indexed pubkey returns error
	bills, err := bs.GetBills(pubKey)
	require.ErrorIs(t, err, ErrPubKeyNotIndexed)
	require.Nil(t, bills)

	// index key
	err = bs.AddKey(&Pubkey{
		Pubkey:     pubKey,
		PubkeyHash: wallet.NewKeyHash(pubKey),
	})
	require.NoError(t, err)

	// verify GetBills for indexed pubkey with no bills returns no error
	bills, err = bs.GetBills(pubKey)
	require.NoError(t, err)
	require.Len(t, bills, 0)

	// add bills
	bill1 := newBillWithValue(1)
	err = bs.SetBills(pubKey, bill1)
	require.NoError(t, err)

	err = bs.SetBills(pubKey, newBillWithValue(2))
	require.NoError(t, err)

	err = bs.SetBills(pubKey, newBillWithValue(3))
	require.NoError(t, err)

	// get bills
	bills, err = bs.GetBills(pubKey)
	require.NoError(t, err)
	require.Len(t, bills, 3)

	// test order number is added to bills
	for i, b := range bills {
		require.EqualValues(t, i+1, b.OrderNumber)
	}

	// test max order number is updated
	var maxOrderNumber uint64
	err = bs.db.View(func(tx *bolt.Tx) error {
		maxOrderNumber = bs.getMaxBillOrderNumber(tx, pubKey)
		return nil
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, maxOrderNumber)

	// test contains bill for pubkey ok
	f, err := bs.ContainsBill(pubKey, bill1.Id)
	require.NoError(t, err)
	require.True(t, f)

	// test contains bill for unknown pubkey nok
	f, err = bs.ContainsBill([]byte{1, 2, 3, 4}, bill1.Id)
	require.NoError(t, err)
	require.False(t, f)

	// test remove bill
	err = bs.RemoveBill(pubKey, bill1.Id)
	require.Nil(t, err)

	// test contains bill returns false after removal
	f, err = bs.ContainsBill(pubKey, bill1.Id)
	require.NoError(t, err)
	require.False(t, f)
}

func TestBillStore_GetSetProofs(t *testing.T) {
	bs, _ := createTestBillStore(t)
	billID := test.NewUnitID(1)
	pubkey := []byte{0}

	// verify GetBill ErrPubKeyNotIndexed
	b, err := bs.GetBill(pubkey, billID)
	require.ErrorIs(t, err, ErrPubKeyNotIndexed)
	require.Nil(t, b)

	// verify SetBills ErrPubKeyNotIndexed
	expectedBill := &Bill{
		Id: billID,
		TxProof: &TxProof{
			BlockNumber: 1,
			Proof:       &block.BlockProof{BlockHeaderHash: []byte{1}},
			Tx:          testtransaction.NewTransaction(t),
		},
	}
	err = bs.SetBills(pubkey, expectedBill)
	require.ErrorIs(t, err, ErrPubKeyNotIndexed)

	// index key
	err = bs.AddKey(&Pubkey{
		Pubkey:     pubkey,
		PubkeyHash: wallet.NewKeyHash(pubkey),
	})
	require.NoError(t, err)

	// verify GetBill ErrBillNotFound
	b, err = bs.GetBill(pubkey, billID)
	require.ErrorIs(t, err, ErrBillNotFound)

	// verify SetBills ok
	err = bs.SetBills(pubkey, expectedBill)
	require.NoError(t, err)

	// verify GetBill ok
	b, err = bs.GetBill(pubkey, billID)
	require.NoError(t, err)
	require.Equal(t, expectedBill, b)
}

func TestBillStore_GetSetKeys(t *testing.T) {
	bs, _ := createTestBillStore(t)
	pubKeyBytes, _ := hexutil.Decode("0x000000000000000000000000000000000000000000000000000000000000000000")
	pubkey := NewPubkey(pubKeyBytes)

	// add key
	err := bs.AddKey(pubkey)
	require.NoError(t, err)

	// verify GetKeys
	keys, err := bs.GetKeys()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, pubkey, keys[0])

	// verify adding same key does not overwrite existing key
	err = bs.AddKey(pubkey)
	require.ErrorIs(t, err, ErrKeyAlreadyExists)
	keys, err = bs.GetKeys()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, pubkey, keys[0])

	// verify GetKey returns key
	key, err := bs.GetKey(pubkey.Pubkey)
	require.NoError(t, err)
	require.Equal(t, pubkey, key)

	// verify GetKey returns nil for unindexed key
	key, err = bs.GetKey([]byte{1, 1, 1, 1})
	require.NoError(t, err)
	require.Nil(t, key)
}

func TestBillStore_DeletingBillForKey1DoesNotAffectKey2(t *testing.T) {
	bs, _ := createTestBillStore(t)
	pk1, _ := hexutil.Decode("0x000000000000000000000000000000000000000000000000000000000000000001")
	pk2, _ := hexutil.Decode("0x000000000000000000000000000000000000000000000000000000000000000002")

	// index keys
	key1 := &Pubkey{
		Pubkey:     pk1,
		PubkeyHash: wallet.NewKeyHash(pk1),
	}
	_ = bs.AddKey(key1)

	key2 := &Pubkey{
		Pubkey:     pk2,
		PubkeyHash: wallet.NewKeyHash(pk2),
	}
	_ = bs.AddKey(key2)

	// add bill to key1 and key2
	bill := newBillWithValue(1)
	_ = bs.SetBills(pk1, bill)
	_ = bs.SetBills(pk2, bill)

	// when bill is removed from key1
	_ = bs.RemoveBill(pk1, bill.Id)

	// then bill should remain for key2
	actualBills, _ := bs.GetBills(pk2)
	require.Len(t, actualBills, 1)
	require.Equal(t, bill, actualBills[0])

	actualBill, err := bs.GetBill(pk2, bill.Id)
	require.NoError(t, err)
	require.Equal(t, bill, actualBill)

	containsBill, err := bs.ContainsBill(pk2, bill.Id)
	require.NoError(t, err)
	require.True(t, containsBill)

	// and key1 bill should be removed
	actualBill, err = bs.GetBill(pk1, bill.Id)
	require.ErrorIs(t, err, ErrBillNotFound)

	containsBill, err = bs.ContainsBill(pk1, bill.Id)
	require.NoError(t, err)
	require.False(t, containsBill)
}

func TestBillStore_DeleteExpiredBills(t *testing.T) {
	s, _ := createTestBillStore(t)
	pubKeyBytes, _ := hexutil.Decode("0x000000000000000000000000000000000000000000000000000000000000000000")
	pubkey := NewPubkey(pubKeyBytes)
	expirationBlockNo := uint64(100)
	unitIDs := [][]byte{{1}, {2}, {3}}
	_ = s.AddKey(pubkey)

	// add three bills and set expiration time
	for _, unitID := range unitIDs {
		err := s.SetBills(pubkey.Pubkey, &Bill{Id: unitID})
		require.NoError(t, err)

		err = s.SetBillExpirationTime(expirationBlockNo, pubkey.Pubkey, unitID)
		require.NoError(t, err)
	}

	// when expiration time is reached
	err := s.DeleteExpiredBills(expirationBlockNo)
	require.NoError(t, err)

	// then expired bills should be deleted
	bills, err := s.GetBills(pubkey.Pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 0)

	// and expired bills metadata should also be cleared
	var expiredBills []*expiredBill
	err = s.db.View(func(tx *bolt.Tx) error {
		var err error
		expiredBills, err = s.getExpiredBills(tx, expirationBlockNo)
		return err
	})
	require.NoError(t, err)
	require.Nil(t, expiredBills)
}

func createTestBillStore(t *testing.T) (*BoltBillStore, error) {
	dbFile := path.Join(t.TempDir(), BoltBillStoreFileName)
	return NewBoltBillStore(dbFile)
}

func newBillWithValue(val uint64) *Bill {
	id := uint256.NewInt(val)
	return &Bill{
		Id:    util.Uint256ToBytes(id),
		Value: val,
	}
}
