package abdrc

import (
	"crypto"
	"fmt"
	"slices"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type StateRequestMsg struct {
	_ struct{} `cbor:",toarray"`
	// ID of the node which requested the state, ie response should
	// be sent to that node
	NodeId string
}

type InputData struct {
	_         struct{} `cbor:",toarray"`
	Partition types.SystemID
	Shard     types.ShardID
	Ir        *types.InputRecord
	Technical certification.TechnicalRecord
	PDRHash   []byte // Partition Description Record Hash
}

type CommittedBlock struct {
	_        struct{} `cbor:",toarray"`
	Block    *drctypes.BlockData
	Ir       []*InputData
	Qc       *drctypes.QuorumCert // block's quorum certificate (from next view)
	CommitQc *drctypes.QuorumCert // commit certificate
}

type ShardInfo struct {
	_         struct{} `cbor:",toarray"`
	Partition types.SystemID
	Shard     types.ShardID
	Round     uint64
	Epoch     uint64
	RootHash  []byte // last certified root hash

	// statistical record of the previous epoch. As we only need
	// it for hashing we keep it in serialized representation
	PrevEpochStat []byte

	// statistical record of the current epoch
	Stat certification.StatisticalRecord

	// per validator total, invariant fees of the previous epoch
	// but as with statistical record of the previous epoch we need it
	// for hashing so we keep it in serialized representation
	PrevEpochFees []byte

	Fees   map[string]uint64 // per validator summary fees of the current epoch
	Leader string            // identifier of the Round leader

	// last CertificationResponse
	UC types.UnicityCertificate
	TR certification.TechnicalRecord
}

type StateMsg struct {
	_             struct{} `cbor:",toarray"`
	ShardInfo     []ShardInfo
	CommittedHead *CommittedBlock
	BlockData     []*drctypes.BlockData
}

/*
CanRecoverToRound returns non-nil error when the state message is not suitable for recovery into round "round".
*/
func (sm *StateMsg) CanRecoverToRound(round uint64) error {
	if sm.CommittedHead == nil {
		return fmt.Errorf("committed block is nil")
	}
	if round < sm.CommittedHead.Block.GetRound() {
		return fmt.Errorf("can't recover to round %d with committed block for round %d", round, sm.CommittedHead.Block.GetRound())
	}
	// commit head matches recover round
	if round == sm.CommittedHead.Block.GetRound() {
		return nil
	}
	if !slices.ContainsFunc(sm.BlockData, func(b *drctypes.BlockData) bool { return b.GetRound() == round }) {
		return fmt.Errorf("state has no data block for round %d", round)
	}

	return nil
}

func (sm *StateMsg) Verify(hashAlgorithm crypto.Hash, tb types.RootTrustBase) error {
	if sm.CommittedHead == nil {
		return fmt.Errorf("commit head is nil")
	}
	if err := sm.CommittedHead.IsValid(); err != nil {
		return fmt.Errorf("invalid commit head: %w", err)
	}
	if err := sm.CommittedHead.Block.Qc.Verify(tb); err != nil {
		return fmt.Errorf("block qc verification error: %w", err)
	}
	if err := sm.CommittedHead.Qc.Verify(tb); err != nil {
		return fmt.Errorf("qc verification error: %w", err)
	}
	if err := sm.CommittedHead.CommitQc.Verify(tb); err != nil {
		return fmt.Errorf("commit qc verification error: %w", err)
	}
	// verify node blocks
	for _, n := range sm.BlockData {
		if err := n.IsValid(); err != nil {
			return fmt.Errorf("invalid block node: %w", err)
		}
		if n.Qc != nil {
			if err := n.Qc.Verify(tb); err != nil {
				return fmt.Errorf("block node qc verification error: %w", err)
			}
		}
	}
	for _, c := range sm.ShardInfo {
		if err := c.UC.Verify(tb, hashAlgorithm, c.UC.UnicityTreeCertificate.SystemIdentifier, c.UC.UnicityTreeCertificate.PartitionDescriptionHash); err != nil {
			return fmt.Errorf("certificate for %s is invalid: %w", c.UC.UnicityTreeCertificate.SystemIdentifier, err)
		}
	}
	return nil
}

func (r *CommittedBlock) GetRound() uint64 {
	if r != nil {
		return r.Block.GetRound()
	}
	return 0
}

func (r *CommittedBlock) IsValid() error {
	if len(r.Ir) == 0 {
		return fmt.Errorf("missing input record state")
	}
	for _, ir := range r.Ir {
		if err := ir.IsValid(); err != nil {
			return fmt.Errorf("invalid input record: %w", err)
		}
	}
	if r.Block == nil {
		return fmt.Errorf("block data is nil")
	}
	if err := r.Block.IsValid(); err != nil {
		return fmt.Errorf("block data error: %w", err)
	}
	if r.Qc == nil {
		return fmt.Errorf("commit head is missing qc certificate")
	}
	if r.CommitQc == nil {
		return fmt.Errorf("commit head is missing commit qc certificate")
	}
	return nil
}

func (i *InputData) IsValid() error {
	if i.Ir == nil {
		return fmt.Errorf("input record is nil")
	}
	if err := i.Ir.IsValid(); err != nil {
		return fmt.Errorf("input record error: %w", err)
	}
	if len(i.PDRHash) == 0 {
		return fmt.Errorf("system description hash not set")
	}
	return nil
}
