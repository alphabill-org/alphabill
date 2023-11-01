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
func (sm *StateMsg) CanRecoverToRound(round uint64, hashAlgorithm crypto.Hash, verifiers map[string]abcrypto.Verifier) error {
	if sm.CommittedHead == nil || sm.CommittedHead.CommitQc == nil {
		return fmt.Errorf("state must have non-nil commit QC")
	}
	if round > sm.CommittedHead.CommitQc.GetRound() {
		return fmt.Errorf("can't recover to round %d with committed QC for round %d", round, sm.CommittedHead.CommitQc.GetRound())
	}

	if !slices.ContainsFunc(sm.BlockNode, func(b *RecoveryBlock) bool { return b.GetRound() == round }) {
		return fmt.Errorf("state has no data block for round %d", round)
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
		return r.Block.Round
	}
	return 0
}
