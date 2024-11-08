package partitions

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

func Test_PartitionStore_Init(t *testing.T) {
	// calling constructor with nil storage should return error
	ps, err := NewPartitionStore(nil)
	require.Nil(t, ps)
	require.EqualError(t, err, `genesis storage must be initialized`)

	// set up genesis store
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	partitions := []*genesis.GenesisPartitionRecord{
		{
			Version: 1,
			PartitionDescription: &types.PartitionDescriptionRecord{
Version: 1,
				NetworkIdentifier:   5,
				PartitionIdentifier: 1,
				T2Timeout:           2600 * time.Millisecond,
			},
			Nodes: []*genesis.PartitionNode{
				{Version: 1, NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
				{Version: 1, NodeIdentifier: "node2", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
				{Version: 1, NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
			},
		},
	}
	gs := &mockGenesisStore{
		partitionRecords: func(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
			require.EqualValues(t, 10, round, "round used to init partition store")
			return partitions, 100, nil
		},
	}

	// calling constructor with non nil storage should return partition store...
	ps, err = NewPartitionStore(gs)
	require.NoError(t, err)
	require.NotNil(t, ps)
	require.Equal(t, uint64(math.MaxUint64), ps.next, "next config change round")
	require.Empty(t, ps.partitions)
	require.Nil(t, ps.curRound)
	//...but to complete the initialization Reset must be called
	err = ps.Reset(func() uint64 { return 10 })
	require.NoError(t, err)
	require.EqualValues(t, 100, ps.next)
	require.NotNil(t, ps.curRound)
	require.Len(t, ps.partitions, 1)

	// Reset might return error when loading from genesis store fail
	expErr := fmt.Errorf("no genesis data")
	gs = &mockGenesisStore{
		partitionRecords: func(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
			return nil, 0, expErr
		},
	}
	ps, err = NewPartitionStore(gs)
	require.NoError(t, err)
	require.NotNil(t, ps)
	err = ps.Reset(func() uint64 { return 10 })
	require.ErrorIs(t, err, expErr)
}

func Test_PartitionStore_AddConfiguration(t *testing.T) {
	t.Run("can't add past configs", func(t *testing.T) {
		gs := &mockGenesisStore{
			addConfiguration: func(round uint64, cfg *genesis.RootGenesis) error {
				return fmt.Errorf("unexpected call (%d, %v)", round, cfg)
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)

		ps.next = 100
		ps.curRound = func() uint64 { return 50 }
		err = ps.AddConfiguration(49, &genesis.RootGenesis{Version: 1})
		require.EqualError(t, err, `can't add config taking effect on round 49 as current round is already 50`)
		require.EqualValues(t, 100, ps.next, "next config round marker mustn't change")
	})

	t.Run("failure to store new config", func(t *testing.T) {
		expErr := fmt.Errorf("save cfg error")
		gs := &mockGenesisStore{
			addConfiguration: func(round uint64, cfg *genesis.RootGenesis) error {
				return expErr
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)

		ps.next = 100
		ps.curRound = func() uint64 { return 50 }
		err = ps.AddConfiguration(51, &genesis.RootGenesis{Version: 1})
		require.ErrorIs(t, err, expErr)
		require.EqualValues(t, 100, ps.next, "next config round marker mustn't change")
	})

	t.Run("success, new config is NOT next", func(t *testing.T) {
		gs := &mockGenesisStore{
			addConfiguration: func(round uint64, cfg *genesis.RootGenesis) error {
				return nil
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)

		ps.next = 100
		ps.curRound = func() uint64 { return 50 }
		err = ps.AddConfiguration(ps.next+1, &genesis.RootGenesis{Version: 1})
		require.NoError(t, err)
		require.EqualValues(t, 100, ps.next, "next config round marker mustn't change")
	})

	t.Run("success, new config is the next", func(t *testing.T) {
		gs := &mockGenesisStore{
			addConfiguration: func(round uint64, cfg *genesis.RootGenesis) error {
				return nil
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)

		ps.next = 100
		ps.curRound = func() uint64 { return 50 }
		err = ps.AddConfiguration(ps.next-1, &genesis.RootGenesis{Version: 1})
		require.NoError(t, err)
		require.EqualValues(t, 99, ps.next, "next config round marker must be updated")
	})
}

func Test_PartitionStore_GetInfo(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	partitions := []*genesis.GenesisPartitionRecord{
		{
			Version: 1,
			PartitionDescription: &types.PartitionDescriptionRecord{
Version: 1,
				NetworkIdentifier:   5,
				PartitionIdentifier: 1,
				T2Timeout:           1000 * time.Millisecond,
			},
			Nodes: []*genesis.PartitionNode{
				{Version: 1, NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
				{Version: 1, NodeIdentifier: "node2", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
				{Version: 1, NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
			},
		},
		{
			Version: 1,
			PartitionDescription: &types.PartitionDescriptionRecord{
Version: 1,
				NetworkIdentifier:   5,
				PartitionIdentifier: 2,
				T2Timeout:           2000 * time.Millisecond,
			},
			Nodes: []*genesis.PartitionNode{
				{Version: 1, NodeIdentifier: "test1", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
				{Version: 1, NodeIdentifier: "test2", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
			},
		},
	}

	t.Run("unknown partition ID", func(t *testing.T) {
		gs := &mockGenesisStore{
			partitionRecords: func(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
				return partitions, 100, nil
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)
		require.NoError(t, ps.Reset(func() uint64 { return 1 }))
		pdr, tb, err := ps.GetInfo(3, 1)
		require.EqualError(t, err, `unknown partition identifier 00000003`)
		require.Nil(t, pdr)
		require.Nil(t, tb)
	})

	t.Run("partition ID exist in current dataset", func(t *testing.T) {
		loadCalls := 0
		gs := &mockGenesisStore{
			// partitionRecords is called each time configuration is loaded from genesis store,
			// we expect it to be called only once, during initial partition store setup
			partitionRecords: func(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
				loadCalls++
				return partitions, 100, nil
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)
		require.NoError(t, ps.Reset(func() uint64 { return 1 }))

		pdr, tb, err := ps.GetInfo(1, 1)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, partitions[0].PartitionDescription, pdr)
		require.EqualValues(t, 1, loadCalls)

		pdr, tb, err = ps.GetInfo(2, 10)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, partitions[1].PartitionDescription, pdr)
		// as the queries hit current dataset no additional calls to genesis store is expected
		require.EqualValues(t, 1, loadCalls)
	})

	t.Run("genesis store returns error on loading data", func(t *testing.T) {
		expErr := fmt.Errorf("no genesis data")
		loadCalls := 0
		gs := &mockGenesisStore{
			partitionRecords: func(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
				loadCalls++
				switch loadCalls {
				case 1:
					return partitions, 100, nil
				default:
					return nil, 0, expErr
				}
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)
		require.NoError(t, ps.Reset(func() uint64 { return 1 }))
		// query from round beyond current range to trigger load from storage
		pdr, tb, err := ps.GetInfo(1, ps.next+1)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, tb)
		require.Nil(t, pdr)
		require.EqualValues(t, 2, loadCalls)
	})

	t.Run("data from genesis store is invalid", func(t *testing.T) {
		invalidPartitions := []*genesis.GenesisPartitionRecord{
			{
				Version: 1,
				PartitionDescription: &types.PartitionDescriptionRecord{
Version: 1,
					NetworkIdentifier:   5,
					PartitionIdentifier: 1,
					T2Timeout:           1000 * time.Millisecond,
				},
				// make one of the PKs invalid so building partition trust base should fail
				Nodes: []*genesis.PartitionNode{
					{Version: 1, NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
					{Version: 1, NodeIdentifier: "node2", SigningPublicKey: []byte{1, 4, 8}, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
					{Version: 1, NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
				},
			},
		}
		loadCalls := 0
		gs := &mockGenesisStore{
			partitionRecords: func(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
				loadCalls++
				switch loadCalls {
				case 1:
					return partitions, 100, nil
				default:
					return invalidPartitions, math.MaxUint64, nil
				}
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)
		require.NoError(t, ps.Reset(func() uint64 { return 1 }))
		// query from round beyond current range to trigger load from storage
		pdr, tb, err := ps.GetInfo(1, ps.next+1)
		require.EqualError(t, err, `switching to new config: creating verifier for the node "node2": pubkey must be 33 bytes long, but is 3`)
		require.Nil(t, tb)
		require.Nil(t, pdr)
		require.EqualValues(t, 2, loadCalls)
	})

	t.Run("successfully load config for next range of rounds", func(t *testing.T) {
		nextConfig := []*genesis.GenesisPartitionRecord{
			{
				Version: 1,
				PartitionDescription: &types.PartitionDescriptionRecord{
Version: 1,
					NetworkIdentifier:   5,
					PartitionIdentifier: 3,
					T2Timeout:           3000 * time.Millisecond,
				},
				Nodes: []*genesis.PartitionNode{
					{Version: 1, NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes, PartitionDescription: types.PartitionDescriptionRecord{Version: 1}},
				},
			},
		}
		loadCalls := 0
		gs := &mockGenesisStore{
			partitionRecords: func(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
				loadCalls++
				switch loadCalls {
				case 1:
					return partitions, 100, nil
				default:
					return nextConfig, math.MaxUint64, nil
				}
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)
		require.NoError(t, ps.Reset(func() uint64 { return 1 }))
		// initial config has partitions 1 and 2
		pdr, tb, err := ps.GetInfo(1, 1)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, partitions[0].PartitionDescription, pdr)
		require.EqualValues(t, 1, loadCalls)

		pdr, tb, err = ps.GetInfo(2, 2)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, partitions[1].PartitionDescription, pdr)
		require.EqualValues(t, 1, loadCalls)
		// but no partition 3
		pdr, tb, err = ps.GetInfo(3, ps.next-1)
		require.EqualError(t, err, `unknown partition identifier 00000003`)
		require.Nil(t, pdr)
		require.Nil(t, tb)

		// query from round beyond current range to trigger load from storage
		// we now should only have partition 3
		cfg2round := ps.next
		pdr, tb, err = ps.GetInfo(1, cfg2round)
		require.EqualError(t, err, `unknown partition identifier 00000001`)
		require.Nil(t, tb)
		require.Nil(t, pdr)
		pdr, tb, err = ps.GetInfo(2, cfg2round+1)
		require.EqualError(t, err, `unknown partition identifier 00000002`)
		require.Nil(t, tb)
		require.Nil(t, pdr)
		pdr, tb, err = ps.GetInfo(3, cfg2round+30)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, nextConfig[0].PartitionDescription, pdr)
		// we expect only two calls to genesis store
		require.EqualValues(t, 2, loadCalls)
	})
}

func Test_TrustBase_Quorum(t *testing.T) {
	// quorum rule is "50% + 1 node"
	var testCases = []struct {
		count  uint64 // node count
		quorum uint64 // quorum value we expect
	}{
		{count: 1, quorum: 1},
		{count: 2, quorum: 2},
		{count: 3, quorum: 2},
		{count: 4, quorum: 3},
		{count: 5, quorum: 3},
		{count: 6, quorum: 4},
		{count: 99, quorum: 50},
		{count: 100, quorum: 51},
	}
	_, verifier := testsig.CreateSignerAndVerifier(t)
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d nodes", tc.count), func(t *testing.T) {
			tb := make(map[string]abcrypto.Verifier, tc.count)
			for v := range tc.count {
				tb[fmt.Sprintf("node%d", v)] = verifier
			}
			ptb := NewPartitionTrustBase(tb)
			require.Equal(t, tc.count, ptb.GetTotalNodes())
			require.Equal(t, tc.quorum, ptb.GetQuorum())
		})
	}
}

func Test_TrustBase_Verify(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	tb := map[string]abcrypto.Verifier{"node1": verifier}
	ptb := NewPartitionTrustBase(tb)

	t.Run("node not found in the trustbase", func(t *testing.T) {
		err := ptb.Verify("foobar", func(v abcrypto.Verifier) error { return fmt.Errorf("unexpected call") })
		require.EqualError(t, err, `node foobar is not part of partition trustbase`)
	})

	t.Run("message does NOT verify", func(t *testing.T) {
		expErr := fmt.Errorf("nope, thats invalid")
		err := ptb.Verify("node1", func(v abcrypto.Verifier) error { return expErr })
		require.ErrorIs(t, err, expErr)
	})

	t.Run("message does verify", func(t *testing.T) {
		require.NoError(t, ptb.Verify("node1", func(v abcrypto.Verifier) error { return nil }))
	})
}

type mockGenesisStore struct {
	addConfiguration func(round uint64, cfg *genesis.RootGenesis) error
	partitionRecords func(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error)
}

func (gs *mockGenesisStore) AddConfiguration(round uint64, cfg *genesis.RootGenesis) error {
	return gs.addConfiguration(round, cfg)
}
func (gs *mockGenesisStore) PartitionRecords(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
	return gs.partitionRecords(round)
}
