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

	t.Run("empty db created", func(t *testing.T) {
		// empty DB
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
}

func testCfg(t *testing.T) *genesis.RootGenesis {
	var cfg *genesis.RootGenesis
	f, err := genesisFiles.Open("testdata/root-genesis-A.json")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(f).Decode(&cfg))
	require.NoError(t, f.Close())
	return cfg
}

func Test_genesisStore_AddConfiguration(t *testing.T) {
	t.Run("invalid config", func(t *testing.T) {
		dbFN := filepath.Join(t.TempDir(), "root.db")
		gs, err := NewGenesisStore(dbFN, &genesis.RootGenesis{})
		require.ErrorContains(t, err, "verifying configuration")
		require.Nil(t, gs)
	})

	t.Run("seed is saved", func(t *testing.T) {
		rgA, rgB := &genesis.RootGenesis{}, &genesis.RootGenesis{}
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
		rg, version, err := gs.GetConfiguration(1)
		require.NoError(t, err)
		require.EqualValues(t, 1, version)
		require.Equal(t, rgA, rg)
		require.NoError(t, gs.db.Close())
	})

	t.Run("success, new config is NOT next", func(t *testing.T) {
		dbFN := filepath.Join(t.TempDir(), "root.db")
		gs, err := NewGenesisStore(dbFN, nil)
		require.NoError(t, err)
		gs.lastUpdate = 50
		gs.nextUpdate = 100

		cfg := testCfg(t)
		err = gs.AddConfiguration(cfg, gs.nextUpdate+1)
		require.NoError(t, err)
		require.EqualValues(t, 100, gs.nextUpdate, "next config round marker mustn't change")
	})

	t.Run("success, new config is next", func(t *testing.T) {
		dbFN := filepath.Join(t.TempDir(), "root.db")
		gs, err := NewGenesisStore(dbFN, nil)
		require.NoError(t, err)
		gs.lastUpdate = 50
		gs.nextUpdate = 100

		cfg := testCfg(t)
		err = gs.AddConfiguration(cfg, gs.nextUpdate+1)
		require.NoError(t, err)
		require.EqualValues(t, 100, gs.nextUpdate, "next config round marker mustn't change")

		err = gs.AddConfiguration(cfg, gs.nextUpdate-1)
		require.NoError(t, err)
		require.EqualValues(t, 99, gs.nextUpdate, "next config round marker must be updated")
	})
}

func Test_genesisStore_loadConfiguration(t *testing.T) {
	var cfgA, cfgB, cfgC *genesis.RootGenesis
	f, err := genesisFiles.Open("testdata/root-genesis-A.json")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(f).Decode(&cfgA))
	require.NoError(t, f.Close())
	f, err = genesisFiles.Open("testdata/root-genesis-B.json")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(f).Decode(&cfgB))
	require.NoError(t, f.Close())
	f, err = genesisFiles.Open("testdata/root-genesis-C.json")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(f).Decode(&cfgC))
	require.NoError(t, f.Close())

	dbFN := filepath.Join(t.TempDir(), "root.db")
	gs, err := NewGenesisStore(dbFN, nil)
	require.NoError(t, err)
	require.NotNil(t, gs)
	// DB must be empty and any query should fail
	rg, _, err := gs.GetConfiguration(3)
	require.EqualError(t, err, `no configuration for round 3, empty DB`)
	require.Nil(t, rg)

	// add configuration for round 100, any query for earlier round should fail
	require.NoError(t, gs.AddConfiguration(cfgB, 100))
	rg, _, err = gs.GetConfiguration(3)
	require.EqualError(t, err, `no configuration for round 3, missing initial genesis`)
	require.Nil(t, rg)

	// but past 100 should return "genesis B"
	cfg, version, err := gs.GetConfiguration(123456)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, uint64(100), version)
	require.Equal(t, uint64(math.MaxUint64), gs.nextUpdate, "round of the next config change")
	require.Equal(t, cfgB, cfg)

	// add initial genesis and another one starting with round 200
	require.NoError(t, gs.AddConfiguration(cfgA, 1))
	require.NoError(t, gs.AddConfiguration(cfgC, 200))
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
		round   uint64               // round number to query
		cfg     *genesis.RootGenesis // expected configuration
		version  uint64              // expected version (first active round)
	}{
		{round: 1, cfg: cfgA, version: 1},
		{round: 2, cfg: cfgA, version: 1},
		{round: 99, cfg: cfgA, version: 1},
		{round: 100, cfg: cfgB, version: 100},
		{round: 101, cfg: cfgB, version: 100},
		{round: 199, cfg: cfgB, version: 100},
		{round: 200, cfg: cfgC, version: 200},
		{round: 201, cfg: cfgC, version: 200},
		{round: 300, cfg: cfgC, version: 200},
		{round: math.MaxUint64, cfg: cfgC, version: 200},
		{round: 150, cfg: cfgB, version: 100},
		{round: 50, cfg: cfgA, version: 1},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("query %d", tc.round), func(t *testing.T) {
			cfg, version, err := gs.GetConfiguration(tc.round)
			require.NoError(t, err)
			require.EqualValues(t, tc.version, version, "returned configuration version")
			require.Equal(t, tc.cfg, cfg)
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
