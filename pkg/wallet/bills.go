package wallet

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/types"
)

type (
	Bills struct {
		Bills []*Bill `json:"bills,omitempty"`
	}

	Bill struct {
		Id                   []byte `json:"id,omitempty"`
		Value                uint64 `json:"value,omitempty,string"`
		TxHash               []byte `json:"txHash,omitempty"`
		DCTargetUnitID       []byte `json:"targetUnitId,omitempty"`
		DCTargetUnitBacklink []byte `json:"targetUnitBacklink,omitempty"`
		Locked               uint64 `json:"locked,omitempty,string"`
	}
)

func NewTxProof(txIdx int, b *types.Block, hashAlgorithm crypto.Hash) (*Proof, error) {
	txProof, txRecord, err := types.NewTxProof(b, txIdx, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return &Proof{
		TxRecord: txRecord,
		TxProof:  txProof,
	}, nil
}

func (x *Bill) GetID() []byte {
	if x != nil {
		return x.Id
	}
	return nil
}

func (x *Bill) GetValue() uint64 {
	if x != nil {
		return x.Value
	}
	return 0
}

func (x *Bill) GetTxHash() []byte {
	if x != nil {
		return x.TxHash
	}
	return nil
}

func (x *Bill) IsLocked() bool {
	if x != nil {
		return x.Locked > 0
	}
	return false
}
