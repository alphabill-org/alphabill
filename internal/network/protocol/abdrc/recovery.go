package abdrc

import (
	abdrc "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/types"
)

type GetStateMsg struct {
	_      struct{} `cbor:",toarray"`
	NodeId string   `json:"nodeId,omitempty"`
}

type InputData struct {
	_     struct{}           `cbor:",toarray"`
	SysID []byte             `json:"sysID,omitempty"`
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

func (r *RecoveryBlock) GetRound() uint64 {
	if r != nil {
		return r.Block.Round
	}
	return 0
}
