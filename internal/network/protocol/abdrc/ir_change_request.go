package abdrc

import (
	"fmt"
	"hash"

	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"

	"github.com/alphabill-org/alphabill/internal/types"
)

type IRChangeReason int

type IRChangeReqMsg struct {
	_           struct{} `cbor:",toarray"`
	IRChangeReq *abtypes.IRChangeReq
}

func (x *IRChangeReqMsg) IsValid() error {
	if x.IRChangeReq == nil {
		return fmt.Errorf("ir change request is missing payload")
	}
	return x.IRChangeReq.IsValid()
}

func (x *IRChangeReqMsg) GetSystemID() types.SystemID {
	if x.IRChangeReq != nil {
		return x.IRChangeReq.SystemIdentifier
	}
	return nil
}

func (x *IRChangeReqMsg) Verify(tb partitions.PartitionTrustBase, luc *types.UnicityCertificate, round, t2InRounds uint64) (*types.InputRecord, error) {
	if err := x.IsValid(); err != nil {
		return nil, fmt.Errorf("ir change request validation failed, %w", err)
	}
	return x.IRChangeReq.Verify(tb, luc, round, t2InRounds)
}

// AddToHasher - add message to hasher. Make sure msg is valid before calling.
func (x *IRChangeReqMsg) AddToHasher(hasher hash.Hash) {
	x.IRChangeReq.AddToHasher(hasher)
}
