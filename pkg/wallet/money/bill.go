package money

import (
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/holiman/uint256"
)

type (
	Bill struct {
		Id      *uint256.Int  `json:"id"`
		Value   uint64        `json:"value,string"`
		TxHash  []byte        `json:"txHash"`
		TxProof *wallet.Proof `json:"txProof"`

		// dc bill specific fields
		TargetUnitID       []byte `json:"targetUnitID"`
		TargetUnitBacklink []byte `json:"targetUnitBacklink"`

		// fcb specific fields
		// LastAddFCTxHash last add fee credit tx hash
		LastAddFCTxHash []byte `json:"lastAddFcTxHash,omitempty"`
	}
)

// GetID returns bill id in 32-byte big endian array
func (b *Bill) GetID() []byte {
	if b != nil {
		return util.Uint256ToBytes(b.Id)
	}
	return nil
}

func (b *Bill) ToGenericBillProof() *wallet.BillProof {
	return &wallet.BillProof{Bill: b.ToGenericBill(), TxProof: b.TxProof}
}

func (b *Bill) ToGenericBill() *wallet.Bill {
	return &wallet.Bill{
		Id:                 b.GetID(),
		Value:              b.Value,
		TxHash:             b.TxHash,
		TargetUnitID:       b.TargetUnitID,
		TargetUnitBacklink: b.TargetUnitBacklink,
		LastAddFCTxHash:    b.LastAddFCTxHash,
	}
}

func (b *Bill) GetTxHash() []byte {
	if b != nil {
		return b.TxHash
	}
	return nil
}

func (b *Bill) GetValue() uint64 {
	if b != nil {
		return b.Value
	}
	return 0
}

func (b *Bill) GetLastAddFCTxHash() []byte {
	if b != nil {
		return b.LastAddFCTxHash
	}
	return nil
}
