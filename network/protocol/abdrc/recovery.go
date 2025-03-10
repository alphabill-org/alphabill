package abdrc

import (
	"crypto"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	rctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type StateRequestMsg struct {
	_ struct{} `cbor:",toarray"`
	// ID of the node which requested the state, ie response should
	// be sent to that node
	NodeId string
}

type CommittedBlock struct {
	_         struct{} `cbor:",toarray"`
	Block     *rctypes.BlockData
	ShardInfo []ShardInfo
	Qc        *rctypes.QuorumCert // block's quorum certificate (from next view)
	CommitQc  *rctypes.QuorumCert // commit certificate
}

type ShardInfo struct {
	_         struct{} `cbor:",toarray"`
	Partition  types.PartitionID
	Shard      types.ShardID
	EpochStart uint64
	T2Timeout  time.Duration
	RootHash   []byte // last certified root hash

	// statistical record of the previous epoch. As we only need
	// it for hashing we keep it in serialized representation
	PrevEpochStat []byte

	// statistical record of the current epoch
	Stat certification.StatisticalRecord

	// per validator total, invariant fees of the previous epoch
	// but as with statistical record of the previous epoch we need it
	// for hashing so we keep it in serialized representation
	PrevEpochFees []byte

	Fees map[string]uint64 // per validator summary fees of the current epoch

	// last CertificationResponse
	UC types.UnicityCertificate
	TR certification.TechnicalRecord

	// input data of the block
	IR            *types.InputRecord
	IRTR          certification.TechnicalRecord
	ShardConfHash []byte
}

type StateMsg struct {
	_             struct{} `cbor:",toarray"`
	CommittedHead *CommittedBlock
	BlockData     []*rctypes.BlockData
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
	if !slices.ContainsFunc(sm.BlockData, func(b *rctypes.BlockData) bool { return b.GetRound() == round }) {
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
	for _, c := range sm.CommittedHead.ShardInfo {
		if err := c.UC.Verify(tb, hashAlgorithm, c.UC.UnicityTreeCertificate.Partition, c.UC.UnicityTreeCertificate.PDRHash); err != nil {
			return fmt.Errorf("certificate for %s is invalid: %w", c.UC.UnicityTreeCertificate.Partition, err)
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
	if len(r.ShardInfo) == 0 {
		return errors.New("missing ShardInfo")
	}
	for _, si := range r.ShardInfo {
		if err := si.IsValid(); err != nil {
			return fmt.Errorf("invalid ShardInfo[%s - %s]: %w", si.Partition, si.Shard, err)
		}
	}
	if r.Block == nil {
		return fmt.Errorf("block data is nil")
	}
	if err := r.Block.IsValid(); err != nil {
		return fmt.Errorf("invalid block data: %w", err)
	}

	if r.Qc == nil {
		return errors.New("commit head is missing qc certificate")
	}
	if r.CommitQc == nil {
		return errors.New("commit head is missing commit qc certificate")
	}
	return nil
}

func (si *ShardInfo) IsValid() error {
	if si.Partition == 0 {
		return errors.New("missing partition id")
	}
	if len(si.PrevEpochStat) == 0 {
		return errors.New("missing PrevEpochStat")
	}
	if len(si.PrevEpochFees) == 0 {
		return errors.New("missing PrevEpochFees")
	}
	if len(si.Fees) == 0 {
		return errors.New("missing Fees")
	}

	if err := si.IR.IsValid(); err != nil {
		return fmt.Errorf("invalid input record: %w", err)
	}
	if len(si.ShardConfHash) == 0 {
		return errors.New("shard conf hash not set")
	}

	if err := si.UC.IsValid(si.Partition, si.ShardConfHash); err != nil {
		return fmt.Errorf("invalid UC: %w", err)
	}
	if err := si.TR.IsValid(); err != nil {
		return fmt.Errorf("invalid TR of CertificationResponse: %w", err)
	}

	return nil
}
