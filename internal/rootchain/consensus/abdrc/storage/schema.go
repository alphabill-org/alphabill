package storage

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
)

const (
	certPrefix  = "cert_"
	blockPrefix = "block_"
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
	VoteMsg  cbor.RawMessage
}

func certKey(id []byte) []byte {
	return append([]byte(certPrefix), id...)
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

	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return fmt.Errorf("cbor encoder init failed: %w", err)
	}
	encoded, err := enc.Marshal(vote)
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
	// not found
	if !found {
		return nil, nil
	}
	switch voteStore.VoteType {
	case VoteMsg:
		var vote abdrc.VoteMsg
		if err = cbor.Unmarshal(voteStore.VoteMsg, &vote); err != nil {
			return nil, fmt.Errorf("vote message deserialization failed: %w", err)
		}
		return &vote, nil
	case TimeoutVoteMsg:
		var vote abdrc.TimeoutMsg
		if err = cbor.Unmarshal(voteStore.VoteMsg, &vote); err != nil {
			return nil, fmt.Errorf("vote message deserialization failed: %w", err)
		}
		return &vote, nil
	}

	return &voteStore, err
}
