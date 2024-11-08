package storage

import (
	"fmt"
	"slices"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

const (
	certPrefix     = "cert_"
	blockPrefix    = "block_"
	timeoutCertKey = "tc"
)
const (
	Unknown VoteType = iota
	VoteMsg
	TimeoutVoteMsg
)

type VoteType uint8

var VoteKey = []byte("vote")

type VoteStore struct {
	VoteType VoteType
	VoteMsg  types.RawCBOR
}

func certKey(id types.PartitionID, shard types.ShardID) []byte {
	return slices.Concat([]byte(certPrefix), id.Bytes(), shard.Bytes())
}

func blockKey(round uint64) []byte {
	return append([]byte(blockPrefix), util.Uint64ToBytes(round)...)
}

func WriteVote(db keyvaluedb.KeyValueDB, vote any) error {
	var voteType VoteType
	switch vote.(type) {
	case *abdrc.VoteMsg:
		voteType = VoteMsg
	case *abdrc.TimeoutMsg:
		voteType = TimeoutVoteMsg
	default:
		return fmt.Errorf("unknown vote type")
	}

	encoded, err := types.Cbor.Marshal(vote)
	if err != nil {
		return fmt.Errorf("vote message serialization failed: %w", err)
	}
	voteStore := &VoteStore{
		VoteType: voteType,
		VoteMsg:  encoded,
	}
	return db.Write(VoteKey, voteStore)
}

func ReadVote(db keyvaluedb.KeyValueDB) (any, error) {
	var voteStore VoteStore
	found, err := db.Read(VoteKey, &voteStore)
	if !found {
		return nil, nil
	}
	// error
	if err != nil {
		return nil, fmt.Errorf("reading vote from db: %w", err)
	}
	switch voteStore.VoteType {
	case VoteMsg:
		var vote abdrc.VoteMsg
		if err = types.Cbor.Unmarshal(voteStore.VoteMsg, &vote); err != nil {
			return nil, fmt.Errorf("vote message deserialization failed: %w", err)
		}
		return &vote, nil
	case TimeoutVoteMsg:
		var vote abdrc.TimeoutMsg
		if err = types.Cbor.Unmarshal(voteStore.VoteMsg, &vote); err != nil {
			return nil, fmt.Errorf("vote message deserialization failed: %w", err)
		}
		return &vote, nil
	}

	return &voteStore, err
}

func WriteLastTC(db keyvaluedb.KeyValueDB, tc *abtypes.TimeoutCert) error {
	return db.Write([]byte(timeoutCertKey), tc)
}

func ReadLastTC(db keyvaluedb.KeyValueDB) (*abtypes.TimeoutCert, error) {
	var tc abtypes.TimeoutCert
	found, err := db.Read([]byte(timeoutCertKey), &tc)
	if !found {
		return nil, nil
	}
	return &tc, err
}
