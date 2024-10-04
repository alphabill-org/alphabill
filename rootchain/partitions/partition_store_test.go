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
	require.EqualError(t, err, `configuration storage must be initialized`)

	// set up configuration store
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	conf := &genesis.RootGenesis{
		Partitions: []*genesis.GenesisPartitionRecord{
		{
			PartitionDescription: &types.PartitionDescriptionRecord{
				SystemIdentifier: 1,
				T2Timeout:        2600 * time.Millisecond,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "node2", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes},
			},
		},
	},
	}
	gs := &mockGenesisStore{
		getConfiguration: func(round uint64) (*genesis.RootGenesis, uint64, error) {
			require.EqualValues(t, 150, round, "round used to init partition store")
			return conf, 100, nil
		},
	}

	// calling constructor with non nil storage should return partition store...
	ps, err = NewPartitionStore(gs)
	require.NoError(t, err)
	require.NotNil(t, ps)
	require.Equal(t, uint64(0), ps.cfgVersion, "loaded configuration version")
	require.Empty(t, ps.partitions)

	_, _, err = ps.GetInfo(1, 150)
	require.NoError(t, err)
	require.EqualValues(t, 100, ps.cfgVersion)
	require.Len(t, ps.partitions, 1)

	// Reset might return error when loading from genesis store fail
	expErr := fmt.Errorf("no genesis data")
	gs = &mockGenesisStore{
		getConfiguration: func(round uint64) (*genesis.RootGenesis, uint64, error) {
			return nil, 0, expErr
		},
	}
	ps, err = NewPartitionStore(gs)
	require.NoError(t, err)
	require.NotNil(t, ps)
	_, _, err = ps.GetInfo(1, 10)
	require.ErrorIs(t, err, expErr)
}

func Test_PartitionStore_GetInfo(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	cfg := &genesis.RootGenesis{
		Partitions: []*genesis.GenesisPartitionRecord{
			{
				PartitionDescription: &types.PartitionDescriptionRecord{
					SystemIdentifier: 1,
					T2Timeout:        1000 * time.Millisecond,
				},
				Nodes: []*genesis.PartitionNode{
					{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
					{NodeIdentifier: "node2", SigningPublicKey: pubKeyBytes},
					{NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes},
				},
			},
			{
				PartitionDescription: &types.PartitionDescriptionRecord{
					SystemIdentifier: 2,
					T2Timeout:        2000 * time.Millisecond,
				},
				Nodes: []*genesis.PartitionNode{
					{NodeIdentifier: "test1", SigningPublicKey: pubKeyBytes},
					{NodeIdentifier: "test2", SigningPublicKey: pubKeyBytes},
				},
			},
		},
	}

	t.Run("unknown partition ID", func(t *testing.T) {
		gs := &mockGenesisStore{
			getConfiguration: func(round uint64) (*genesis.RootGenesis, uint64, error) {
				return cfg, 100, nil
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)
		sdr, tb, err := ps.GetInfo(3, 1)
		require.EqualError(t, err, `unknown partition identifier 00000003`)
		require.Nil(t, sdr)
		require.Nil(t, tb)
	})

	t.Run("partition ID exist in current dataset", func(t *testing.T) {
		gs := &mockGenesisStore{
			// partitionRecords is called each time configuration is loaded from genesis store,
			// we expect it to be called only once, during initial partition store setup
			getConfiguration: func(round uint64) (*genesis.RootGenesis, uint64, error) {
				return cfg, 100, nil
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)

		sdr, tb, err := ps.GetInfo(1, 1)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, cfg.Partitions[0].PartitionDescription, sdr)

		sdr, tb, err = ps.GetInfo(2, 10)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, cfg.Partitions[1].PartitionDescription, sdr)
	})

	t.Run("genesis store returns error on loading data", func(t *testing.T) {
		expErr := fmt.Errorf("no genesis data")
		gs := &mockGenesisStore{
			getConfiguration: func(round uint64) (*genesis.RootGenesis, uint64, error) {
				if round >= 100 {
					return cfg, 100, nil
				}
				return nil, 0, expErr
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)
		sdr, tb, err := ps.GetInfo(1, 100)
		require.NoError(t, err)

		sdr, tb, err = ps.GetInfo(1, 99)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, tb)
		require.Nil(t, sdr)
	})

	t.Run("data from genesis store is invalid", func(t *testing.T) {
		cfgWithInvalidPartitions := &genesis.RootGenesis{
			Partitions: []*genesis.GenesisPartitionRecord{
				{
					PartitionDescription: &types.PartitionDescriptionRecord{
						SystemIdentifier: 1,
						T2Timeout:        1000 * time.Millisecond,
					},
					// make one of the PKs invalid so building partition trust base should fail
					Nodes: []*genesis.PartitionNode{
						{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
						{NodeIdentifier: "node2", SigningPublicKey: []byte{1, 4, 8}},
						{NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes},
					},
				},
			},
		}
		gs := &mockGenesisStore{
			getConfiguration: func(round uint64) (*genesis.RootGenesis, uint64, error) {
				if round < 100 {
					return cfg, 1, nil
				}
				return cfgWithInvalidPartitions, math.MaxUint64, nil
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)
		// query from round beyond current range to trigger load from storage
		sdr, tb, err := ps.GetInfo(1, 101)
		require.EqualError(t, err, `loading new configuration: creating verifier for the node "node2": pubkey must be 33 bytes long, but is 3`)
		require.Nil(t, tb)
		require.Nil(t, sdr)
	})

	t.Run("successfully load config for next range of rounds", func(t *testing.T) {
		nextCfg := &genesis.RootGenesis{
			Partitions: []*genesis.GenesisPartitionRecord{
				{
					PartitionDescription: &types.PartitionDescriptionRecord{
						SystemIdentifier: 3,
						T2Timeout:        3000 * time.Millisecond,
					},
					Nodes: []*genesis.PartitionNode{
						{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
					},
				},
			},
		}
		nextCfgRound := uint64(200)
		gs := &mockGenesisStore{
			getConfiguration: func(round uint64) (*genesis.RootGenesis, uint64, error) {
				if round < 200 {
					return cfg, 1, nil
				}
				return nextCfg, nextCfgRound, nil
			},
		}
		ps, err := NewPartitionStore(gs)
		require.NoError(t, err)

		// initial config has partitions 1 and 2
		sdr, tb, err := ps.GetInfo(1, 1)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, cfg.Partitions[0].PartitionDescription, sdr)

		sdr, tb, err = ps.GetInfo(2, 2)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, cfg.Partitions[1].PartitionDescription, sdr)

		// but no partition 3
		sdr, tb, err = ps.GetInfo(3, 199)
		require.EqualError(t, err, `unknown partition identifier 00000003`)
		require.Nil(t, sdr)
		require.Nil(t, tb)

		// query from round beyond current range to trigger load from storage
		// we now should only have partition 3
		sdr, tb, err = ps.GetInfo(1, nextCfgRound)
		require.EqualError(t, err, `unknown partition identifier 00000001`)
		require.Nil(t, tb)
		require.Nil(t, sdr)
		sdr, tb, err = ps.GetInfo(2, nextCfgRound+1)
		require.EqualError(t, err, `unknown partition identifier 00000002`)
		require.Nil(t, tb)
		require.Nil(t, sdr)
		sdr, tb, err = ps.GetInfo(3, nextCfgRound+30)
		require.NoError(t, err)
		require.NotNil(t, tb)
		require.Equal(t, nextCfg.Partitions[0].PartitionDescription, sdr)
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
		mv := mockMsgVerification{
			isValid: func(v abcrypto.Verifier) error { return fmt.Errorf("unexpected call") },
		}
		err := ptb.Verify("foobar", mv)
		require.EqualError(t, err, `node foobar is not part of partition trustbase`)
	})

	t.Run("message does NOT verify", func(t *testing.T) {
		expErr := fmt.Errorf("nope, thats invalid")
		mv := mockMsgVerification{
			isValid: func(v abcrypto.Verifier) error { return expErr },
		}
		err := ptb.Verify("node1", mv)
		require.ErrorIs(t, err, expErr)
	})

	t.Run("message does verify", func(t *testing.T) {
		mv := mockMsgVerification{
			isValid: func(v abcrypto.Verifier) error { return nil },
		}
		require.NoError(t, ptb.Verify("node1", mv))
	})
}

type mockGenesisStore struct {
	addConfiguration func(cfg *genesis.RootGenesis, round uint64) error
	getConfiguration func(round uint64) (*genesis.RootGenesis, uint64, error)
}

func (gs *mockGenesisStore) AddConfiguration(cfg *genesis.RootGenesis, round uint64) error {
	return gs.addConfiguration(cfg, round)
}
func (gs *mockGenesisStore) GetConfiguration(round uint64) (*genesis.RootGenesis, uint64, error) {
	return gs.getConfiguration(round)
}

type mockMsgVerification struct {
	isValid func(v abcrypto.Verifier) error
}

func (mv mockMsgVerification) IsValid(v abcrypto.Verifier) error { return mv.isValid(v) }
