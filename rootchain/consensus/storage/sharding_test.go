package storage

import (
	"crypto"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
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
		}
		si := ShardInfo{
			Fees: make(map[string]uint64),
			LastCR: &certification.CertificationResponse{
				Technical: certification.TechnicalRecord{Leader: "L"},
			},
		}
		si.update(cr)

		require.EqualValues(t, cr.InputRecord.RoundNumber, si.Round)
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
		si.update(cr)
		require.EqualValues(t, 2000, si.Stat.MaxBlockSize)
		require.EqualValues(t, 2000, si.Stat.MaxStateSize)
		require.EqualValues(t, 2000, si.Stat.MaxFee)

		// max values must change
		cr.BlockSize = 3001
		cr.StateSize = 3002
		cr.InputRecord.SumOfEarnedFees = 3003
		si.update(cr)
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
		si.update(cr)
		require.Zero(t, si.Stat.Blocks)

		// state changes, should count the block
		cr.InputRecord.Hash = append(cr.InputRecord.Hash, 0)
		si.update(cr)
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
		Round:    3432,
		Epoch:    3,
		RootHash: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2},
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
		RoundNumber:  si.Round,
		Epoch:        si.Epoch,
		Hash:         si.RootHash,
		PreviousHash: si.RootHash,
		BlockHash:    make([]byte, 32),
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
			Partition:      si.LastCR.Partition,
			Shard:          si.LastCR.Shard,
			NodeIdentifier: "1111",
			InputRecord: &types.InputRecord{
				Version:      1,
				RoundNumber:  si.Round + 1, // incoming request must be for next round
				Epoch:        si.Epoch,
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

		bcr.NodeIdentifier = "unknown"
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
		bcr.Partition++
		require.NoError(t, bcr.Sign(signer))
		require.EqualError(t, si.ValidRequest(bcr), `request of shard 00000017- but ShardInfo of 00000016-`)

		bcr = validBCR()
		bcr.Shard, _ = si.LastCR.Shard.Split()
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
	pdr := types.PartitionDescriptionRecord{
		PartitionIdentifier: 8,
	}
	irEpoch1 := types.InputRecord{
		RoundNumber: 100,
		Epoch:       2,
		Hash:        []byte{1, 2, 3, 4, 5, 6, 7, 8},
		Timestamp:   20241114100,
	}

	zH := make([]byte, 32)
	ucE1 := testcertificates.CreateUnicityCertificate(t, signer, &irEpoch1, &pdr, 1, zH, zH)
	// to keep the UC hash deterministic so we can check that expected leader was selected
	ucE1.UnicitySeal.Timestamp = 1000000
	ucE1.UnicitySeal.Signatures = nil

	// returns shard info in the end of epoch 1
	getSI := func(t *testing.T) ShardInfo {
		si := ShardInfo{
			Round: irEpoch1.RoundNumber,
			Epoch: irEpoch1.Epoch,
			Fees:  map[string]uint64{"B": 2, "A": 1, "C": 3},
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
				Partition: pdr.PartitionIdentifier,
				UC:        *ucE1,
			},
			nodeIDs: []string{"A", "B", "C"},
		}
		require.NoError(t, si.LastCR.SetTechnicalRecord(certification.TechnicalRecord{
			Round:    si.Round + 1,
			Epoch:    si.Epoch,
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
		orc := mockOrchestration{
			shardEpoch: func(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error) {
				return si.Epoch, nil
			},
		}
		rootH := si.RootHash
		// no BCR, ie timeout round
		tr, err := si.nextRound(nil, orc)
		require.NoError(t, err)
		require.Equal(t, si.Round+1, tr.Round, "TR is for the next round")
		require.Equal(t, si.Epoch, tr.Epoch, "epoch mustn't have changed")
		require.Equal(t, "B", tr.Leader)
		// stat and fee hash calculated based on si
		h, err := si.statHash(crypto.SHA256)
		require.NoError(t, err)
		require.EqualValues(t, h, tr.StatHash)
		h, err = si.feeHash(crypto.SHA256)
		require.NoError(t, err)
		require.EqualValues(t, h, tr.FeeHash)
		// as BCR was nil stat and root hash mustn't change
		require.Equal(t, rootH, si.RootHash)
	})

	t.Run("next epoch", func(t *testing.T) {
		// case where next round is in the next epoch
		zH := make([]byte, 32)
		irE2 := irEpoch1
		irE2.Epoch++
		pgEpoch2 := &genesis.GenesisPartitionRecord{
			Version:     1,
			Certificate: testcertificates.CreateUnicityCertificate(t, signer, &irE2, &pdr, 900, zH, zH),
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "2222", SigningPublicKey: pubKey},
			},
			PartitionDescription: &pdr,
		}

		si := getSI(t)
		orc := mockOrchestration{
			shardEpoch: func(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error) {
				if round > si.Round {
					return si.Epoch + 1, nil
				}
				return si.Epoch, nil
			},
			shardConfig: func(partition types.PartitionID, shard types.ShardID, epoch uint64) (*genesis.GenesisPartitionRecord, error) {
				return pgEpoch2, nil
			},
		}
		rootH := si.RootHash
		// no BCR, ie timeout round
		tr, err := si.nextRound(nil, orc)
		require.NoError(t, err)
		require.Equal(t, si.Round+1, tr.Round, "TR is for the next round")
		require.Equal(t, si.Epoch+1, tr.Epoch, "epoch must have changed")
		require.Equal(t, "2222", tr.Leader, "leader must be from the next epoch")
		// stat and fee hash calculated based on si of the next epoch
		// we just check that it isn't equal to the hash based on current si
		h, err := si.statHash(crypto.SHA256)
		require.NoError(t, err)
		require.NotEqualValues(t, h, tr.StatHash)
		h, err = si.feeHash(crypto.SHA256)
		require.NoError(t, err)
		require.NotEqualValues(t, h, tr.FeeHash)
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
	pdr := types.PartitionDescriptionRecord{PartitionIdentifier: 7}
	pgEpoch2 := &genesis.GenesisPartitionRecord{
		Version: 1,
		Nodes: []*genesis.PartitionNode{
			{NodeIdentifier: "2222", SigningPublicKey: validKey},
		},
		Certificate:          testcertificates.CreateUnicityCertificate(t, signer, ir, &pdr, 1, zH, zH),
		PartitionDescription: &pdr,
	}

	orc := mockOrchestration{
		shardEpoch: func(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error) {
			if round > 100 {
				return 2, nil
			}
			return 1, nil
		},
		shardConfig: func(partition types.PartitionID, shard types.ShardID, epoch uint64) (*genesis.GenesisPartitionRecord, error) {
			return pgEpoch2, nil
		},
	}

	// shard info in the end of epoch 1
	ucE1 := testcertificates.CreateUnicityCertificate(t, signer, ir, &pdr, 1, zH, zH)
	si := ShardInfo{
		Round: 100,
		Epoch: 1,
		Fees:  map[string]uint64{"B": 2, "A": 1, "C": 3},
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
			Partition: pdr.PartitionIdentifier,
			UC:        *ucE1,
		},
		nodeIDs: []string{"A", "B", "C"},
	}
	require.NoError(t, si.LastCR.SetTechnicalRecord(certification.TechnicalRecord{
		Round:    si.Round,
		Leader:   "A",
		StatHash: []byte{5},
		FeeHash:  []byte{0xF},
	}))
	require.NoError(t, si.IsValid())

	// when block is extended si.nextRound is called to get TR for the
	// Input Change Request to be certified
	tr, err := si.nextRound(nil, orc)
	require.NoError(t, err)
	// when block is committed CertResponse is created based on that
	// and assigned to SI.LastCR
	trH, err := tr.Hash()
	require.NoError(t, err)
	ucE1 = testcertificates.CreateUnicityCertificate(t, signer, ir, &pdr, 1, si.RootHash, trH)
	si.LastCR.Technical = tr
	si.LastCR.UC = *ucE1
	require.NoError(t, si.IsValid())

	// epoch switch hasn't happened yet but the TR should already have
	// next round, next epoch & leader from validator set of the next epoch
	require.Equal(t, si.Round+1, si.LastCR.Technical.Round)
	require.Equal(t, si.Epoch+1, si.LastCR.Technical.Epoch)
	require.Equal(t, "2222", si.LastCR.Technical.Leader)

	// when processing next block proposal ShardInfo of the previous
	// round is cloned and si.nextEpoch is called for shards where
	// si.Epoch != si.LastCR.Technical.Epoch ie last CertResp
	// triggered epoch change
	nextSI, err := si.nextEpoch(pgEpoch2)
	require.NoError(t, err)
	require.NotNil(t, nextSI)
	require.NoError(t, nextSI.IsValid())
	// data which is carried on to the next epoch
	require.Equal(t, si.RootHash, nextSI.RootHash)
	require.Equal(t, si.Round, nextSI.Round)
	require.Equal(t, si.LastCR, nextSI.LastCR)
	require.Equal(t, si.Epoch+1, nextSI.Epoch)
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
	signer, _ := testsig.CreateSignerAndVerifier(t)
	validKey := []byte{0x3, 0x24, 0x8b, 0x61, 0x68, 0x51, 0xac, 0x6e, 0x43, 0x7e, 0xc2, 0x4e, 0xcc, 0x21, 0x9e, 0x5b, 0x42, 0x43, 0xdf, 0xa5, 0xdb, 0xdb, 0x8, 0xce, 0xa6, 0x48, 0x3a, 0xc9, 0xe0, 0xdc, 0x6b, 0x55, 0xcd}
	ir := &types.InputRecord{
		RoundNumber: 900,
		Epoch:       1,
		Hash:        []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	pdr := &types.PartitionDescriptionRecord{PartitionIdentifier: 7}
	zH := make([]byte, 32)
	pgEpoch1 := &genesis.GenesisPartitionRecord{
		Version: 1,
		Nodes: []*genesis.PartitionNode{
			{NodeIdentifier: "1111", SigningPublicKey: validKey},
		},
		Certificate:          testcertificates.CreateUnicityCertificate(t, signer, ir, pdr, 1, zH, zH),
		PartitionDescription: pdr,
	}

	t.Run("success", func(t *testing.T) {
		si, err := NewShardInfoFromGenesis(pgEpoch1)
		require.NoError(t, err)
		require.Equal(t, pgEpoch1.Certificate.GetRoundNumber(), si.Round)
		require.Equal(t, pgEpoch1.Certificate.InputRecord.Epoch, si.Epoch)
		require.EqualValues(t, pgEpoch1.Certificate.InputRecord.Hash, si.RootHash)
		require.Equal(t, certification.StatisticalRecord{}, si.Stat)
		require.Equal(t, map[string]uint64{"1111": 0}, si.Fees)
		require.Equal(t, types.RawCBOR{0xA0}, si.PrevEpochFees)
		require.Equal(t, types.RawCBOR{0x87, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, si.PrevEpochStat)
		require.Equal(t, "1111", si.LastCR.Technical.Leader)
		require.Equal(t, si.Round+1, si.LastCR.Technical.Round)
		require.Equal(t, si.Epoch, si.LastCR.Technical.Epoch)
	})

	t.Run("no nodes", func(t *testing.T) {
		pg := *pgEpoch1
		pg.Nodes = nil
		si, err := NewShardInfoFromGenesis(&pg)
		require.EqualError(t, err, `creating TechnicalRecord: node list is empty`)
		require.Empty(t, si)
	})

	t.Run("invalid key", func(t *testing.T) {
		pg := *pgEpoch1
		pg.Nodes = []*genesis.PartitionNode{
			{NodeIdentifier: "1111", SigningPublicKey: []byte{1, 2, 3}},
		}
		si, err := NewShardInfoFromGenesis(&pg)
		require.EqualError(t, err, `shard info init: creating verifier for the node "1111": pubkey must be 33 bytes long, but is 3`)
		require.Empty(t, si)
	})
}

func Test_shardStates_nextBlock(t *testing.T) {
	t.Run("no epoch changes", func(t *testing.T) {
		orc := mockOrchestration{}
		si := ShardInfo{
			Round:  22,
			Fees:   map[string]uint64{"A": 0},
			LastCR: &certification.CertificationResponse{Technical: certification.TechnicalRecord{}},
		}
		shardKey := partitionShard{1, types.ShardID{}.Key()}
		ssA := shardStates{shardKey: &si}
		ssB, err := ssA.nextBlock(orc)
		require.NoError(t, err)
		require.Equal(t, ssA, ssB, "expected clone to be identical")

		// modifying clone should not modify the original
		si.Fees["B"] = 1
		require.NotEqual(t, ssA, ssB)
	})

	t.Run("epoch change, missing config", func(t *testing.T) {
		expErr := errors.New("nope, don't have this config")
		orc := mockOrchestration{
			shardConfig: func(partition types.PartitionID, shard types.ShardID, epoch uint64) (*genesis.GenesisPartitionRecord, error) {
				return nil, expErr
			},
		}
		si := ShardInfo{
			Epoch: 1,
			Round: 22,
			Fees:  map[string]uint64{"A": 0},
			LastCR: &certification.CertificationResponse{
				Technical: certification.TechnicalRecord{
					Epoch: 2,
				},
			},
		}
		shardKey := partitionShard{1, types.ShardID{}.Key()}
		ssA := shardStates{shardKey: &si}
		ssB, err := ssA.nextBlock(orc)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, ssB)
	})

	t.Run("epoch change", func(t *testing.T) {
		// test that ShardInfo.nextEpoch is called - validating that the returned state is
		// correct "clone" of the current state is tested by the SI.nextEpoch tests
		orc := mockOrchestration{
			// return genesis where Epoch number is not +1 of the current one - this causes
			// known error we can test against to make sure that SI.nextEpoch was called
			shardConfig: func(partition types.PartitionID, shard types.ShardID, epoch uint64) (*genesis.GenesisPartitionRecord, error) {
				return &genesis.GenesisPartitionRecord{
					Certificate: &types.UnicityCertificate{
						InputRecord: &types.InputRecord{Epoch: 3},
					},
				}, nil
			},
		}
		si := ShardInfo{
			Epoch: 1,
			Round: 22,
			Fees:  map[string]uint64{"A": 0},
			LastCR: &certification.CertificationResponse{
				Partition: 1,
				Technical: certification.TechnicalRecord{
					Epoch: 2,
				},
			},
		}
		shardKey := partitionShard{1, types.ShardID{}.Key()}
		ssA := shardStates{shardKey: &si}
		ssB, err := ssA.nextBlock(orc)
		require.EqualError(t, err, `creating ShardInfo 00000001 -  of the next epoch: epochs must be consecutive, current is 1 proposed next 3`)
		require.Nil(t, ssB)
	})
}

type mockOrchestration struct {
	shardEpoch  func(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error)
	shardConfig func(partition types.PartitionID, shard types.ShardID, epoch uint64) (*genesis.GenesisPartitionRecord, error)
}

func (mo mockOrchestration) ShardEpoch(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error) {
	return mo.shardEpoch(partition, shard, round)
}

func (mo mockOrchestration) ShardConfig(partition types.PartitionID, shard types.ShardID, epoch uint64) (*genesis.GenesisPartitionRecord, error) {
	return mo.shardConfig(partition, shard, epoch)
}
