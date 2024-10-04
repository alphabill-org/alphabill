package rootchain

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

func Test_shardInfo_Update(t *testing.T) {
	t.Run("values do get assigned", func(t *testing.T) {
		cr := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{
				RoundNumber:     80940,
				Epoch:           1,
				Hash:            []byte{2, 6, 7, 8, 9, 0, 1},
				PreviousHash:    []byte{1, 1, 1, 1},
				BlockHash:       []byte{6, 6, 6, 6, 6, 6},
				SumOfEarnedFees: 1900,
			},
			BlockSize: 2222,
			StateSize: 3333,
			Leader:    "1234567890",
		}
		si := shardInfo{Fees: make(map[string]uint64)}
		si.Update(cr)

		require.EqualValues(t, cr.InputRecord.RoundNumber, si.Round)
		require.EqualValues(t, cr.InputRecord.Hash, si.RootHash)
		require.EqualValues(t, cr.InputRecord.SumOfEarnedFees, si.Fees[cr.Leader])
		require.EqualValues(t, cr.InputRecord.SumOfEarnedFees, si.Stat.BlockFees)
		require.EqualValues(t, cr.BlockSize, si.Stat.BlockSize)
		require.EqualValues(t, cr.StateSize, si.Stat.StateSize)
		require.EqualValues(t, cr.BlockSize, si.Stat.MaxBlockSize)
		require.EqualValues(t, cr.StateSize, si.Stat.MaxStateSize)
		require.EqualValues(t, cr.InputRecord.SumOfEarnedFees, si.Stat.MaxFee)
	})

	t.Run("stat max values updated correctly", func(t *testing.T) {
		si := shardInfo{
			Fees: make(map[string]uint64),
			Stat: certification.StatisticalRecord{
				MaxFee:       2000,
				MaxBlockSize: 2000,
				MaxStateSize: 2000,
			},
		}
		// max values mustn't change
		cr := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{
				SumOfEarnedFees: 1001,
			},
			BlockSize: 1002,
			StateSize: 1003,
		}
		si.Update(cr)
		require.EqualValues(t, 2000, si.Stat.MaxBlockSize)
		require.EqualValues(t, 2000, si.Stat.MaxStateSize)
		require.EqualValues(t, 2000, si.Stat.MaxFee)

		// max values must change
		cr.BlockSize = 3001
		cr.StateSize = 3002
		cr.InputRecord.SumOfEarnedFees = 3003
		si.Update(cr)
		require.EqualValues(t, 3001, si.Stat.MaxBlockSize)
		require.EqualValues(t, 3002, si.Stat.MaxStateSize)
		require.EqualValues(t, 3003, si.Stat.MaxFee)
	})

	t.Run("counting blocks", func(t *testing.T) {
		si := shardInfo{Fees: make(map[string]uint64)}

		// state didn't change, block count should stay zero
		cr := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{
				Hash:         []byte{1, 1, 1, 1},
				PreviousHash: []byte{1, 1, 1, 1},
			},
		}
		si.Update(cr)
		require.Zero(t, si.Stat.Blocks)

		// state changes, should count the block
		cr.InputRecord.Hash = append(cr.InputRecord.Hash, 0)
		si.Update(cr)
		require.EqualValues(t, 1, si.Stat.Blocks)
	})
}

func Test_shardInfo_feeBytes(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		require.Empty(t, (&shardInfo{}).feeBytes())
		require.Empty(t, (&shardInfo{Fees: map[string]uint64{}}).feeBytes())
	})

	t.Run("output is sorted by node ID", func(t *testing.T) {
		si := &shardInfo{
			Fees: map[string]uint64{
				"2":  2,
				"3":  3,
				"1":  1,
				"0":  0,
				"10": 10,
			},
		}

		require.Equal(t,
			[]byte{
				'0', 0, 0, 0, 0, 0, 0, 0, 0,
				'1', 0, 0, 0, 0, 0, 0, 0, 1,
				'1', '0', 0, 0, 0, 0, 0, 0, 0, 10,
				'2', 0, 0, 0, 0, 0, 0, 0, 2,
				'3', 0, 0, 0, 0, 0, 0, 0, 3,
			},
			si.feeBytes(),
		)
	})
}
