package storage

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
)

const hashAlg = crypto.SHA256

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
		}
		si := ShardInfo{
			Fees: make(map[string]uint64),
			LastCR: &certification.CertificationResponse{
				Technical: certification.TechnicalRecord{Leader: "L"},
			},
		}
		si.update(cr, "L")

		require.EqualValues(t, cr.InputRecord.Hash, si.RootHash)
		require.EqualValues(t, cr.InputRecord.SumOfEarnedFees, si.Fees[si.LastCR.Technical.Leader])
		require.EqualValues(t, cr.InputRecord.SumOfEarnedFees, si.Stat.BlockFees)
		require.EqualValues(t, cr.BlockSize, si.Stat.BlockSize)
		require.EqualValues(t, cr.StateSize, si.Stat.StateSize)
		require.EqualValues(t, cr.BlockSize, si.Stat.MaxBlockSize)
		require.EqualValues(t, cr.StateSize, si.Stat.MaxStateSize)
		require.EqualValues(t, cr.InputRecord.SumOfEarnedFees, si.Stat.MaxFee)
	})

	t.Run("stat max values updated correctly", func(t *testing.T) {
		si := ShardInfo{
			Fees: make(map[string]uint64),
			Stat: certification.StatisticalRecord{
				MaxFee:       2000,
				MaxBlockSize: 2000,
				MaxStateSize: 2000,
			},
			LastCR: &certification.CertificationResponse{},
		}
		// max values mustn't change
		cr := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{
				SumOfEarnedFees: 1001,
			},
			BlockSize: 1002,
			StateSize: 1003,
		}
		si.update(cr, "L")
		require.EqualValues(t, 2000, si.Stat.MaxBlockSize)
		require.EqualValues(t, 2000, si.Stat.MaxStateSize)
		require.EqualValues(t, 2000, si.Stat.MaxFee)

		// max values must change
		cr.BlockSize = 3001
		cr.StateSize = 3002
		cr.InputRecord.SumOfEarnedFees = 3003
		si.update(cr, "L")
		require.EqualValues(t, 3001, si.Stat.MaxBlockSize)
		require.EqualValues(t, 3002, si.Stat.MaxStateSize)
		require.EqualValues(t, 3003, si.Stat.MaxFee)
	})

	t.Run("counting blocks", func(t *testing.T) {
		si := ShardInfo{Fees: make(map[string]uint64), LastCR: &certification.CertificationResponse{}}

		// state didn't change, block count should stay zero
		cr := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{
				Hash:         []byte{1, 1, 1, 1},
				PreviousHash: []byte{1, 1, 1, 1},
			},
		}
		si.update(cr, "L")
		require.Zero(t, si.Stat.Blocks)

		// state changes, should count the block
		cr.InputRecord.Hash = append(cr.InputRecord.Hash, 0)
		si.update(cr, "L")
		require.EqualValues(t, 1, si.Stat.Blocks)
	})
}

func Test_ShardInfo_ValidRequest(t *testing.T) {
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)

	// shard info we test against
	si := &ShardInfo{
		PartitionID: 22,
		ShardID:     types.ShardID{},
		RootHash:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2},
		LastCR: &certification.CertificationResponse{
			Partition: 22,
			Shard:     types.ShardID{},
			UC: types.UnicityCertificate{
				Version: 1,
				UnicitySeal: &types.UnicitySeal{
					Version:              1,
					RootChainRoundNumber: 5555555,
					Timestamp:            types.NewTimestamp(),
				},
			},
		},
		trustBase: map[string]abcrypto.Verifier{"1111": verifier},
	}
	si.LastCR.UC.InputRecord = &types.InputRecord{
		Version:      1,
		RoundNumber:  3432,
		Epoch:        3,
		Hash:         si.RootHash,
		PreviousHash: si.RootHash,
		BlockHash:    nil,
		SummaryValue: []byte{5, 5, 5},
		Timestamp:    20241113,
	}
	require.NoError(t,
		si.LastCR.SetTechnicalRecord(certification.TechnicalRecord{
			Epoch:  si.LastCR.UC.InputRecord.Epoch,
			Round:  si.LastCR.UC.InputRecord.RoundNumber + 1,
			Leader: "1111",
		}))

	// return BCR which is valid next request for "si" above (but not signed)
	validBCR := func() *certification.BlockCertificationRequest {
		return &certification.BlockCertificationRequest{
			PartitionID: si.LastCR.Partition,
			ShardID:     si.LastCR.Shard,
			NodeID:      "1111",
			InputRecord: &types.InputRecord{
				Version:      1,
				RoundNumber:  si.LastCR.Technical.Round, // incoming request must be for next round
				Epoch:        si.LastCR.Technical.Epoch,
				PreviousHash: si.RootHash,
				Hash:         []byte{2, 2, 2, 2, 2, 6, 6, 6, 6, 6},
				BlockHash:    []byte{1},
				SummaryValue: []byte{2},
				Timestamp:    si.LastCR.UC.UnicitySeal.Timestamp,
			},
		}
	}

	t.Run("signature", func(t *testing.T) {
		bcr := validBCR()
		require.NoError(t, bcr.Sign(signer))
		require.NoError(t, si.ValidRequest(bcr))
		// changing some property should invalidate the signature
		bcr.InputRecord.RoundNumber++
		require.EqualError(t, si.ValidRequest(bcr), `invalid certification request: signature verification: verification failed`)

		bcr.NodeID = "unknown"
		require.EqualError(t, si.ValidRequest(bcr), `invalid certification request: node "unknown" is not in the trustbase of the shard`)
	})

	t.Run("round number", func(t *testing.T) {
		bcr := validBCR()
		bcr.InputRecord.RoundNumber++
		require.NoError(t, bcr.Sign(signer))
		require.EqualError(t, si.ValidRequest(bcr), `expected round 3433, got 3434`)
	})

	t.Run("epoch", func(t *testing.T) {
		bcr := validBCR()
		bcr.InputRecord.Epoch++
		require.NoError(t, bcr.Sign(signer))
		require.EqualError(t, si.ValidRequest(bcr), `expected epoch 3, got 4`)
	})

	t.Run("root hash", func(t *testing.T) {
		bcr := validBCR()
		bcr.InputRecord.PreviousHash = []byte{0}
		require.NoError(t, bcr.Sign(signer))
		require.EqualError(t, si.ValidRequest(bcr), `request has different root hash for last certified state`)
	})

	t.Run("wrong shard", func(t *testing.T) {
		bcr := validBCR()
		bcr.PartitionID++
		require.NoError(t, bcr.Sign(signer))
		require.EqualError(t, si.ValidRequest(bcr), `request of shard 00000017- but ShardInfo of 00000016-`)

		bcr = validBCR()
		bcr.ShardID, _ = si.LastCR.Shard.Split()
		require.NoError(t, bcr.Sign(signer))
		require.EqualError(t, si.ValidRequest(bcr), `request of shard 00000016-0 but ShardInfo of 00000016-`)
	})

	t.Run("IR.IsValid is called", func(t *testing.T) {
		bcr := validBCR()
		bcr.InputRecord.Version = 0
		require.NoError(t, bcr.Sign(signer))
		require.EqualError(t, si.ValidRequest(bcr), `invalid certification request: invalid input record: invalid version (type *types.InputRecord)`)
	})
}

func Test_ShardInfo_nextRound(t *testing.T) {
	pubKey := []byte{0x3, 0x24, 0x8b, 0x61, 0x68, 0x51, 0xac, 0x6e, 0x43, 0x7e, 0xc2, 0x4e, 0xcc, 0x21, 0x9e, 0x5b, 0x42, 0x43, 0xdf, 0xa5, 0xdb, 0xdb, 0x8, 0xce, 0xa6, 0x48, 0x3a, 0xc9, 0xe0, 0xdc, 0x6b, 0x55, 0xcd}
	signer, _ := testsig.CreateSignerAndVerifier(t)
	shardConf := types.PartitionDescriptionRecord{
		PartitionID: 8,
		ShardID:     types.ShardID{},
		Epoch:       2,
		EpochStart:  1,
	}
	irEpoch1 := types.InputRecord{
		RoundNumber: 100,
		Epoch:       2,
		Hash:        []byte{1, 2, 3, 4, 5, 6, 7, 8},
		Timestamp:   20241114100,
	}

	zH := make([]byte, 32)
	ucE1 := testcertificates.CreateUnicityCertificate(t, signer, &irEpoch1, &shardConf, 1, zH, zH)

	// returns shard info in the end of epoch 1
	getSI := func(t *testing.T) ShardInfo {
		si := ShardInfo{
			PartitionID: shardConf.PartitionID,
			ShardID:     shardConf.ShardID,
			RootHash:    []byte{0, 1, 2, 3, 4, 5, 6, 7},
			Fees:        map[string]uint64{"B": 2, "A": 1, "C": 3},
			Stat: certification.StatisticalRecord{
				Blocks:       0,
				BlockFees:    1,
				BlockSize:    2,
				StateSize:    3,
				MaxFee:       4,
				MaxBlockSize: 5,
				MaxStateSize: 6,
			},
			LastCR: &certification.CertificationResponse{
				Partition: shardConf.PartitionID,
				Shard:     shardConf.ShardID,
				UC:        *ucE1,
			},
			TR:      certification.TechnicalRecord{Leader: "A", Round: irEpoch1.RoundNumber, Epoch: irEpoch1.Epoch},
			nodeIDs: []string{"A", "B", "C"},
		}
		require.NoError(t, si.LastCR.SetTechnicalRecord(certification.TechnicalRecord{
			Round:    irEpoch1.RoundNumber + 1,
			Epoch:    irEpoch1.Epoch,
			Leader:   "A",
			StatHash: []byte{5},
			FeeHash:  []byte{0xF},
		}))
		require.NoError(t, si.IsValid())
		return si
	}

	t.Run("same epoch", func(t *testing.T) {
		// case where next round is in the same epoch
		si := getSI(t)
		rootH := si.RootHash
		// no BCR, ie timeout round
		prevTR := si.TR
		err := si.nextRound(nil, &shardConf, hashAlg)
		require.NoError(t, err)
		require.Equal(t, prevTR.Round+1, si.TR.Round, "TR is for the next round")
		require.Equal(t, prevTR.Epoch, si.TR.Epoch, "epoch mustn't have changed")
		require.Equal(t, "C", si.TR.Leader)
		// stat and fee hash calculated based on si
		h, err := si.statHash(hashAlg)
		require.NoError(t, err)
		require.EqualValues(t, h, si.TR.StatHash)
		h, err = si.feeHash(hashAlg)
		require.NoError(t, err)
		require.EqualValues(t, h, si.TR.FeeHash)
		// as BCR was nil stat and root hash mustn't change
		require.Equal(t, rootH, si.RootHash)
	})

	t.Run("next epoch", func(t *testing.T) {
		// case where next round is in the next epoch
		irE2 := irEpoch1
		irE2.Epoch++
		varEpoch2 := &types.PartitionDescriptionRecord{
			NetworkID:   0,
			PartitionID: 7,
			ShardID:     types.ShardID{},
			Epoch:       3,
			EpochStart:  101,
			Validators: []*types.NodeInfo{
				{
					NodeID: "2222",
					SigKey: pubKey,
					Stake:  1,
				},
			},
		}

		si := getSI(t)
		rootH := si.RootHash
		prevTR := si.TR
		// no BCR, ie timeout round
		err := si.nextRound(nil, varEpoch2, hashAlg)
		require.NoError(t, err)
		require.Equal(t, prevTR.Round+1, si.TR.Round, "TR is for the next round")
		require.Equal(t, prevTR.Epoch+1, si.TR.Epoch, "epoch must have changed")
		require.Equal(t, "2222", si.TR.Leader, "leader must be from the next epoch")
		// stat and fee hash calculated based on si of the next epoch
		// we just check that it isn't equal to the hash based on current si
		h, err := si.statHash(hashAlg)
		require.NoError(t, err)
		require.NotEqualValues(t, h, si.TR.StatHash)
		h, err = si.feeHash(hashAlg)
		require.NoError(t, err)
		require.NotEqualValues(t, h, si.TR.FeeHash)
		// as BCR was nil stat and root hash mustn't change
		require.Equal(t, rootH, si.RootHash)
	})
}

func Test_ShardInfo_NextEpoch(t *testing.T) {
	validKey := []byte{0x3, 0x24, 0x8b, 0x61, 0x68, 0x51, 0xac, 0x6e, 0x43, 0x7e, 0xc2, 0x4e, 0xcc, 0x21, 0x9e, 0x5b, 0x42, 0x43, 0xdf, 0xa5, 0xdb, 0xdb, 0x8, 0xce, 0xa6, 0x48, 0x3a, 0xc9, 0xe0, 0xdc, 0x6b, 0x55, 0xcd}
	zH := make([]byte, 32)
	signer, _ := testsig.CreateSignerAndVerifier(t)
	ir := &types.InputRecord{
		RoundNumber: 101,
		Epoch:       2,
		Hash:        []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	pdr := types.PartitionDescriptionRecord{PartitionID: 7}
	varEpoch2 := &types.PartitionDescriptionRecord{
		NetworkID:   0,
		PartitionID: 7,
		ShardID:     types.ShardID{},
		Epoch:       2,
		EpochStart:  101,
		Validators: []*types.NodeInfo{
			{
				NodeID: "2222",
				SigKey: validKey,
				Stake:  1,
			},
		},
	}

	// shard info in the end of epoch 1
	ucE1 := testcertificates.CreateUnicityCertificate(t, signer, ir, &pdr, 1, zH, zH)
	si := ShardInfo{
		RootHash: []byte{0, 1, 2, 3, 4, 5, 6, 7},
		Fees:     map[string]uint64{"B": 2, "A": 1, "C": 3},
		Stat: certification.StatisticalRecord{
			Blocks:       0,
			BlockFees:    1,
			BlockSize:    2,
			StateSize:    3,
			MaxFee:       4,
			MaxBlockSize: 5,
			MaxStateSize: 6,
		},
		LastCR: &certification.CertificationResponse{
			Partition: pdr.PartitionID,
			UC:        *ucE1,
		},
		TR:      certification.TechnicalRecord{Leader: "A", Round: 100, Epoch: 1},
		nodeIDs: []string{"A", "B", "C"},
	}
	require.NoError(t, si.LastCR.SetTechnicalRecord(certification.TechnicalRecord{
		Round:    100,
		Leader:   "A",
		StatHash: []byte{5},
		FeeHash:  []byte{0xF},
	}))
	require.NoError(t, si.IsValid())

	// when block is extended si.nextRound is called to get TR for the
	// certificate generated by this block (which shard will use for next round)
	lastTR := si.TR
	err := si.nextRound(nil, varEpoch2, hashAlg)
	require.NoError(t, err)
	require.Equal(t, lastTR.Round+1, si.TR.Round)
	require.Equal(t, lastTR.Epoch+1, si.TR.Epoch)
	require.Equal(t, "2222", si.TR.Leader, "expected leader form the next epoch TB")
	// when block is committed CertResponse is created based on that
	// and assigned to SI.LastCR
	trH, err := si.TR.Hash()
	require.NoError(t, err)
	ucE1 = testcertificates.CreateUnicityCertificate(t, signer, ir, &pdr, 1, si.RootHash, trH)
	si.LastCR.Technical = si.TR
	si.LastCR.UC = *ucE1
	require.NoError(t, si.IsValid())

	// when processing next block proposal ShardInfo of the previous round
	// is cloned and si.nextEpoch is called for shards where epoch change
	nextSI, err := si.nextEpoch(varEpoch2, hashAlg)
	require.NoError(t, err)
	require.NotNil(t, nextSI)
	require.NoError(t, nextSI.IsValid())
	require.Equal(t, lastTR.Epoch+1, nextSI.TR.Epoch)
	// data which is carried on to the next epoch
	require.Equal(t, si.RootHash, nextSI.RootHash)
	require.Equal(t, si.LastCR, nextSI.LastCR)
	/* Fee list of the previous epoch was serialized
	A3       # map(3)
	   61    # text(1)
	      41 # "A"
	   01    # unsigned(1)
	   61    # text(1)
	      42 # "B"
	   02    # unsigned(2)
	   61    # text(1)
	      43 # "C"
	   03    # unsigned(3)
	*/
	require.Equal(t, types.RawCBOR{0xa3, 0x61, 0x41, 0x1, 0x61, 0x42, 0x2, 0x61, 0x43, 0x3}, nextSI.PrevEpochFees)
	// fee list is initialized to new validator list
	require.Equal(t, map[string]uint64{"2222": 0}, nextSI.Fees)
	// array of 7 items, sorted by field order in the struct
	require.Equal(t, types.RawCBOR{0x87, 0, 1, 2, 3, 4, 5, 6}, nextSI.PrevEpochStat)
	require.Equal(t, certification.StatisticalRecord{}, nextSI.Stat, "expected stat to be reset")
}

func Test_ShardInfo_Quorum(t *testing.T) {
	// GetQuorum depends on the items in the trustbase
	si := ShardInfo{}
	require.EqualValues(t, 0, si.GetTotalNodes())
	require.EqualValues(t, 1, si.GetQuorum())

	si.trustBase = map[string]abcrypto.Verifier{}
	require.EqualValues(t, 0, si.GetTotalNodes())
	require.EqualValues(t, 1, si.GetQuorum())

	si.trustBase["1"] = nil // using nil as actual value is not important in this case
	require.EqualValues(t, 1, si.GetTotalNodes())
	require.EqualValues(t, 1, si.GetQuorum())

	si.trustBase["2"] = nil
	require.EqualValues(t, 2, si.GetTotalNodes())
	require.EqualValues(t, 2, si.GetQuorum())

	si.trustBase["3"] = nil
	require.EqualValues(t, 3, si.GetTotalNodes())
	require.EqualValues(t, 2, si.GetQuorum())

	si.trustBase["4"] = nil
	require.EqualValues(t, 4, si.GetTotalNodes())
	require.EqualValues(t, 3, si.GetQuorum())
}

func Test_NewShardInfoFromGenesis(t *testing.T) {
	validKey := []byte{0x3, 0x24, 0x8b, 0x61, 0x68, 0x51, 0xac, 0x6e, 0x43, 0x7e, 0xc2, 0x4e, 0xcc, 0x21, 0x9e, 0x5b, 0x42, 0x43, 0xdf, 0xa5, 0xdb, 0xdb, 0x8, 0xce, 0xa6, 0x48, 0x3a, 0xc9, 0xe0, 0xdc, 0x6b, 0x55, 0xcd}
	nodeID, _ := testutils.RandomNodeID(t)
	shardConf := &types.PartitionDescriptionRecord{
		PartitionID: 7,
		Validators: []*types.NodeInfo{
			{
				NodeID: nodeID,
				SigKey: validKey,
				Stake:  1,
			},
		},
	}

	t.Run("success", func(t *testing.T) {
		si, err := NewShardInfo(shardConf, hashAlg)
		require.NoError(t, err)
		require.Nil(t, si.RootHash)
		require.Nil(t, si.LastCR)
		require.Equal(t, certification.StatisticalRecord{}, si.Stat)
		require.Equal(t, map[string]uint64{nodeID: 0}, si.Fees)
		require.Equal(t, types.RawCBOR{0xA0}, si.PrevEpochFees)
		require.Equal(t, types.RawCBOR{0x87, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, si.PrevEpochStat)
	})
}

func Test_shardStates_nextBlock(t *testing.T) {
	t.Run("new shard", func(t *testing.T) {
		// configuration contains new shard (no info on the current state)
		si := ShardInfo{
			Fees: map[string]uint64{"A": 0},
			LastCR: &certification.CertificationResponse{
				Partition: 1,
			},
			IR: &types.InputRecord{},
		}
		pdr2 := newShardConf(t)
		pdr2.PartitionID = si.LastCR.Partition + 1
		psID := types.PartitionShardID{PartitionID: si.LastCR.Partition, ShardID: si.LastCR.Shard.Key()}
		psID2 := types.PartitionShardID{PartitionID: pdr2.PartitionID, ShardID: pdr2.ShardID.Key()}
		shardConfs := map[types.PartitionShardID]*types.PartitionDescriptionRecord{psID: {}, psID2: pdr2}

		ssA := ShardStates{States: map[types.PartitionShardID]*ShardInfo{psID: &si}}
		ssB, err := ssA.nextBlock(shardConfs, hashAlg)
		require.NoError(t, err)
		require.Len(t, ssB.States, 2)
		require.Contains(t, ssB.Changed, psID2, `new shard must be in the "changed" list`)
	})

	t.Run("no epoch changes", func(t *testing.T) {
		si := ShardInfo{
			PartitionID: 1,
			ShardID:     types.ShardID{},
			Fees:        map[string]uint64{"A": 0},
			LastCR: &certification.CertificationResponse{
				Partition: 1,
				Shard:     types.ShardID{},
			},
			IR: &types.InputRecord{Epoch: 1},
			TR: certification.TechnicalRecord{Epoch: 1},
		}
		psID := types.PartitionShardID{PartitionID: si.LastCR.Partition, ShardID: si.LastCR.Shard.Key()}
		shardConfs := map[types.PartitionShardID]*types.PartitionDescriptionRecord{psID: {}}

		ssA := ShardStates{States: map[types.PartitionShardID]*ShardInfo{psID: &si}, Changed: ShardSet{}}
		ssB, err := ssA.nextBlock(shardConfs, hashAlg)
		require.NoError(t, err)
		require.Equal(t, ssA, ssB, "expected clone to be identical")
		require.Empty(t, ssB.Changed)

		// modifying clone should not modify the original
		si.Fees["B"] = 1
		require.NotEqual(t, ssA, ssB)
	})

	t.Run("epoch change", func(t *testing.T) {
		// test that ShardInfo.nextEpoch is called - validating that the returned state is
		// correct "clone" of the current state is tested by the SI.nextEpoch tests
		si := ShardInfo{
			PartitionID: 1,
			ShardID:     types.ShardID{},
			Fees:        map[string]uint64{"A": 0},
			LastCR: &certification.CertificationResponse{
				Partition: 1,
				Shard:     types.ShardID{},
			},
			IR: &types.InputRecord{Epoch: 1},
			TR: certification.TechnicalRecord{Epoch: 2},
		}
		psID := types.PartitionShardID{PartitionID: si.PartitionID, ShardID: si.ShardID.Key()}
		// return genesis where Epoch number is not +1 of the current one - this causes
		// known error we can test against to make sure that SI.nextEpoch was called
		shardConfs := map[types.PartitionShardID]*types.PartitionDescriptionRecord{psID: {Epoch: 3}}

		ssA := ShardStates{States: map[types.PartitionShardID]*ShardInfo{psID: &si}}
		_, err := ssA.nextBlock(shardConfs, hashAlg)
		require.EqualError(t, err, `creating ShardInfo 00000001 -  of the next epoch: epochs must be consecutive, expected 2 proposed next 3`)
	})
}

func Test_ShardInfo_selectLeader(t *testing.T) {
	signer, _ := testsig.CreateSignerAndVerifier(t)
	pdr := types.PartitionDescriptionRecord{
		PartitionID: 8,
	}
	irEpoch1 := types.InputRecord{
		RoundNumber: 100,
		Epoch:       2,
		Hash:        []byte{1, 2, 3, 4, 5, 6, 7, 8},
		Timestamp:   20241114100,
	}
	ucE1 := testcertificates.CreateUnicityCertificate(t, signer, &irEpoch1, &pdr, 1, nil, nil)

	t.Run("root hash is nil", func(t *testing.T) {
		si := ShardInfo{
			RootHash: nil,
			Fees:     map[string]uint64{"B": 2, "A": 1, "C": 3},
			Stat: certification.StatisticalRecord{
				Blocks:       0,
				BlockFees:    1,
				BlockSize:    2,
				StateSize:    3,
				MaxFee:       4,
				MaxBlockSize: 5,
				MaxStateSize: 6,
			},
			LastCR: &certification.CertificationResponse{
				Partition: pdr.PartitionID,
				UC:        *ucE1,
			},
			nodeIDs: []string{"A", "B", "C"},
		}
		expectedLeaderID := si.selectLeader(irEpoch1.RoundNumber)
		require.Equal(t, "B", expectedLeaderID)
	})
}

type mockOrchestration struct {
	shardConfigs func(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error)
	shardConfig  func(partitionID types.PartitionID, shardID types.ShardID, rootRound uint64) (*types.PartitionDescriptionRecord, error)
}

func (mo mockOrchestration) NetworkID() types.NetworkID {
	return 5
}

func (mo mockOrchestration) ShardConfigs(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error) {
	return mo.shardConfigs(rootRound)
}

func (mo mockOrchestration) ShardConfig(partitionID types.PartitionID, shardID types.ShardID, rootRound uint64) (*types.PartitionDescriptionRecord, error) {
	return mo.shardConfig(partitionID, shardID, rootRound)
}
