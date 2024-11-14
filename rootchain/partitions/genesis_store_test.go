package partitions

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

func Test_NewGenesisStore(t *testing.T) {
	t.Run("directory not exist", func(t *testing.T) {
		// attempt to create file in a not existing directory should fail
		dbFN := filepath.Join(t.TempDir(), "notExist", "root.db")
		gs, err := NewGenesisStore(dbFN, nil)
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, gs)
	})

	t.Run("seed is nil", func(t *testing.T) {
		// empty DB but nil seed will result in empty DB
		dbFN := filepath.Join(t.TempDir(), "root.db")
		gs, err := NewGenesisStore(dbFN, nil)
		require.NoError(t, err)
		require.NotNil(t, gs)
		require.NoError(t, gs.db.View(
			func(tx *bbolt.Tx) error {
				if k, v := tx.Bucket(rootGenesisBucket).Cursor().First(); k != nil || v != nil {
					return fmt.Errorf("bucket is not empty")
				}
				return nil
			}),
		)
		require.NoError(t, gs.db.Close())
	})

	t.Run("seed is saved", func(t *testing.T) {
		rgA, rgB := &genesis.RootGenesis{Version: 1}, &genesis.RootGenesis{Version: 1}
		f, err := genesisFiles.Open("testdata/root-genesis-A.json")
		require.NoError(t, err)
		require.NoError(t, json.NewDecoder(f).Decode(rgA))
		require.NoError(t, f.Close())
		f, err = genesisFiles.Open("testdata/root-genesis-B.json")
		require.NoError(t, err)
		require.NoError(t, json.NewDecoder(f).Decode(rgB))
		require.NoError(t, f.Close())

		dbFN := filepath.Join(t.TempDir(), "root.db")
		gs, err := NewGenesisStore(dbFN, rgA)
		require.NoError(t, err)
		require.NotNil(t, gs)
		require.NoError(t, gs.db.View(
			func(tx *bbolt.Tx) error {
				b := tx.Bucket(rootGenesisBucket)
				if k, v := b.Cursor().First(); k == nil || v == nil {
					return fmt.Errorf("bucket is empty")
				}
				if stat := b.Stats(); stat.KeyN != 1 || stat.BucketN != 1 {
					return fmt.Errorf("expected 1 bucket (got %d) and 1 key/value (got %d)", stat.BucketN, stat.KeyN)
				}
				return nil
			}),
		)
		require.NoError(t, gs.db.Close())

		// if we now reopen the DB with different seed the original
		// data must be preserved and new seed ignored
		gs, err = NewGenesisStore(dbFN, rgB)
		require.NoError(t, err)
		require.NotNil(t, gs)
		require.NoError(t, gs.db.View(
			func(tx *bbolt.Tx) error {
				b := tx.Bucket(rootGenesisBucket)
				if k, v := b.Cursor().First(); k == nil || v == nil {
					return fmt.Errorf("bucket is empty")
				}
				if stat := b.Stats(); stat.KeyN != 1 || stat.BucketN != 1 {
					return fmt.Errorf("expected 1 bucket (got %d) and 1 key/value (got %d)", stat.BucketN, stat.KeyN)
				}
				return nil
			}),
		)
		rg, next, err := gs.activeGenesis(1)
		require.NoError(t, err)
		if next != math.MaxUint64 {
			t.Errorf("expected next to be max uint64, got %d", next)
		}
		require.Equal(t, rgA, rg)
		require.NoError(t, gs.db.Close())
	})
}

func Test_genesisStore_activeGenesis(t *testing.T) {
	var rgA, rgB, rgC *genesis.RootGenesis
	f, err := genesisFiles.Open("testdata/root-genesis-A.json")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(f).Decode(&rgA))
	require.NoError(t, f.Close())
	f, err = genesisFiles.Open("testdata/root-genesis-B.json")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(f).Decode(&rgB))
	require.NoError(t, f.Close())
	f, err = genesisFiles.Open("testdata/root-genesis-C.json")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(f).Decode(&rgC))
	require.NoError(t, f.Close())

	dbFN := filepath.Join(t.TempDir(), "root.db")
	gs, err := NewGenesisStore(dbFN, nil)
	require.NoError(t, err)
	require.NotNil(t, gs)
	// we used nil seed so DB must be empty and any query should fail
	rg, _, err := gs.activeGenesis(3)
	require.EqualError(t, err, `no configuration for round 3, empty DB`)
	require.Nil(t, rg)

	// add genesis for round 100, any query for earlier round should fail
	require.NoError(t, gs.AddConfiguration(100, rgB))
	rg, _, err = gs.activeGenesis(3)
	require.EqualError(t, err, `no configuration for round 3, missing initial genesis`)
	require.Nil(t, rg)
	// but past 100 should return "genesis B"
	rg, next, err := gs.activeGenesis(123456)
	require.NoError(t, err)
	require.Equal(t, uint64(math.MaxUint64), next, "round of the next config change")
	require.Equal(t, rgB, rg)

	// add initial genesis and another one starting with round 200
	require.NoError(t, gs.AddConfiguration(1, rgA))
	require.NoError(t, gs.AddConfiguration(200, rgC))
	// verify that we have DB in expected state
	require.NoError(t, gs.db.View(
		func(tx *bbolt.Tx) error {
			b := tx.Bucket(rootGenesisBucket)
			if k, v := b.Cursor().First(); k == nil || v == nil {
				return fmt.Errorf("bucket is empty")
			}
			if stat := b.Stats(); stat.KeyN != 3 || stat.BucketN != 1 {
				return fmt.Errorf("expected 1 bucket (got %d) and 3 key/value (got %d)", stat.BucketN, stat.KeyN)
			}
			return nil
		}),
	)

	var testCases = []struct {
		query uint64               // round number to query
		rg    *genesis.RootGenesis // expected genesis data
		next  uint64               // expected next config change round
	}{
		{query: 1, rg: rgA, next: 100},
		{query: 2, rg: rgA, next: 100},
		{query: 99, rg: rgA, next: 100},
		{query: 100, rg: rgB, next: 200},
		{query: 101, rg: rgB, next: 200},
		{query: 199, rg: rgB, next: 200},
		{query: 200, rg: rgC, next: math.MaxUint64},
		{query: 201, rg: rgC, next: math.MaxUint64},
		{query: 300, rg: rgC, next: math.MaxUint64},
		{query: math.MaxUint64, rg: rgC, next: math.MaxUint64},
		{query: 150, rg: rgB, next: 200},
		{query: 50, rg: rgA, next: 100},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("query %d", tc.query), func(t *testing.T) {
			rg, next, err := gs.activeGenesis(tc.query)
			require.NoError(t, err)
			require.EqualValues(t, tc.next, next, "round of the next config change")
			require.Equal(t, tc.rg, rg)
		})
	}

	require.NoError(t, gs.db.Close())
}

func Test_roundKey(t *testing.T) {
	for _, v := range []uint64{0, 1, 0x0200, 0x030000, 0x04030201, 0xFEDCBA9876543210, math.MaxUint64 - 1, math.MaxUint64} {
		if r := keyToRound(roundToKey(v)); r != v {
			t.Errorf("expected %d, got %d", v, r)
		}
	}
}

//go:embed testdata/*.json
var genesisFiles embed.FS
