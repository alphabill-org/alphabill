package abdrc

import (
	"crypto"
	"fmt"
	"slices"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	abdrc "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/types"
)

type GetStateMsg struct {
	_ struct{} `cbor:",toarray"`
	// ID of the node which requested the state, ie response should
	// be sent to that node
	NodeId string `json:"nodeId,omitempty"`
}

type InputData struct {
	_     struct{}           `cbor:",toarray"`
	SysID types.SystemID32   `json:"sysID,omitempty"`
	Ir    *types.InputRecord `json:"ir,omitempty"`
	Sdrh  []byte             `json:"sdrh,omitempty"`
}

type RecoveryBlock struct {
	_        struct{}          `cbor:",toarray"`
	Block    *abdrc.BlockData  `json:"block,omitempty"`
	Ir       []*InputData      `json:"ir,omitempty"`
	Qc       *abdrc.QuorumCert `json:"qc,omitempty"`
	CommitQc *abdrc.QuorumCert `json:"commitQc,omitempty"`
}

type StateMsg struct {
	_             struct{}                    `cbor:",toarray"`
	Certificates  []*types.UnicityCertificate `json:"certificates,omitempty"`
	CommittedHead *RecoveryBlock              `json:"committedHead,omitempty"`
	BlockNode     []*RecoveryBlock            `json:"blockNode,omitempty"`
}

/*
CanRecoverToRound returns non-nil error when the state message is not suitable for recovery into round "round".
*/
func (sm *StateMsg) CanRecoverToRound(round uint64) error {
	if sm.CommittedHead == nil || sm.CommittedHead.CommitQc == nil {
		return fmt.Errorf("state must have non-nil commit QC")
	}
	if round < sm.CommittedHead.GetRound() {
		return fmt.Errorf("can't recover to round %d with committed block for round %d", round, sm.CommittedHead.GetRound())
	}
	// commit head matches recover round
	if sm.CommittedHead.GetRound() == round {
		return nil
	}
	if !slices.ContainsFunc(sm.BlockNode, func(b *RecoveryBlock) bool { return b.GetRound() == round }) {
		return fmt.Errorf("state has no data block for round %d", round)
	}

	return nil
}

func (sm *StateMsg) Verify(hashAlgorithm crypto.Hash, quorum uint32, verifiers map[string]abcrypto.Verifier) error {
	if sm.CommittedHead == nil {
		return fmt.Errorf("commit head is nil")
	}
	if err := sm.CommittedHead.IsValid(); err != nil {
		return fmt.Errorf("invalid commit head block: %w", err)
	}
	if sm.CommittedHead.CommitQc == nil {
		return fmt.Errorf("invalid commit head, commit qc is nil")
	}
	if err := sm.CommittedHead.CommitQc.Verify(quorum, verifiers); err != nil {
		return fmt.Errorf("invalid commit head, commit qc error: %w", err)
	}
	if sm.CommittedHead.Qc != nil {
		if err := sm.CommittedHead.Qc.Verify(quorum, verifiers); err != nil {
			return fmt.Errorf("commit head qc verification error: %w", err)
		}
	}
	// verify node blocks
	for _, n := range sm.BlockNode {
		if err := n.IsValid(); err != nil {
			return fmt.Errorf("invalid block node: %w", err)
		}
		if n.CommitQc != nil {
			return fmt.Errorf("invalid block node, has commit qc set")
		}
		if n.Qc != nil {
			if err := n.Qc.Verify(quorum, verifiers); err != nil {
				return fmt.Errorf("block node qc verification error: %w", err)
			}
		}
	}
	for _, c := range sm.Certificates {
		if err := c.IsValid(verifiers, hashAlgorithm, c.UnicityTreeCertificate.SystemIdentifier, c.UnicityTreeCertificate.SystemDescriptionHash); err != nil {
			return fmt.Errorf("certificate for %X is invalid: %w", c.UnicityTreeCertificate.SystemIdentifier, err)
		}
	}
	return nil
}

func (r *RecoveryBlock) GetRound() uint64 {
	if r != nil {
		return r.Block.GetRound()
	}
	return 0
}

func (r *RecoveryBlock) IsValid() error {
	if len(r.Ir) == 0 {
		return fmt.Errorf("invalid commit head, missing input record state")
	}
	for _, ir := range r.Ir {
		if err := ir.IsValid(); err != nil {
			return fmt.Errorf("invalid input record: %w", err)
		}
	}
	if r.Block == nil {
		return fmt.Errorf("invalid commit head, block data is nil")
	}
	if err := r.Block.IsValid(); err != nil {
		return fmt.Errorf("invalid commit head, block data error: %w", err)
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
	if len(i.Sdrh) == 0 {
		return fmt.Errorf("system descrition hash not set")
	}
	return nil
}
