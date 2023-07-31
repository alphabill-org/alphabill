package wallet

import (
	"math"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_storage(t *testing.T) {
	t.Parallel()

	// testing things in one bucket only ie can (re)use the same db
	db := initTestStorage(t)

	t.Run("trying to open the same DB doesn't hang", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			storage, err := newBoltStore(db.db.Path())
			require.EqualError(t, err, `failed to open bolt DB: timeout`)
			require.Nil(t, storage)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("test didn't complete within timeout")
		}
	})

	t.Run("block number", func(t *testing.T) {
		testBlockNumber(t, db)
	})

	t.Run("fee credits", func(t *testing.T) {
		testFeeCredits(t, db)
	})
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

func testFeeCredits(t *testing.T, db *storage) {
	// nil key returns nil
	fcb, err := db.GetFeeCreditBill(nil)
	require.NoError(t, err)
	require.Nil(t, fcb)

	// unknown key returns nil
	fcb, err = db.GetFeeCreditBill([]byte{0})
	require.NoError(t, err)
	require.Nil(t, fcb)

	// set fee credit bills
	fcbs := []*FeeCreditBill{
		{Id: []byte{1}, Value: 1, TxHash: []byte{1}, LastAddFCTxHash: []byte{1}},
		{Id: []byte{2}, Value: 2, TxHash: []byte{2}, LastAddFCTxHash: []byte{2}},
		{Id: []byte{3}, Value: 3, TxHash: []byte{3}, LastAddFCTxHash: []byte{3}},
	}
	for _, b := range fcbs {
		err = db.SetFeeCreditBill(b)
		require.NoError(t, err)
	}

	// get fee credit bills
	for _, expectedFCB := range fcbs {
		actualFCB, err := db.GetFeeCreditBill(expectedFCB.Id)
		require.NoError(t, err)
		require.Equal(t, expectedFCB, actualFCB)
	}
}

func initTestStorage(t *testing.T) *storage {
	t.Helper()
	store, err := newBoltStore(filepath.Join(t.TempDir(), "vd.db"))
	require.NoError(t, err)
	require.NotNil(t, store)

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("%s: store.Close returned error: %v", t.Name(), err)
		}
	})

	return store
}
