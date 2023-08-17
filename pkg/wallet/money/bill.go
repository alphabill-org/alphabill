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
		DcTimeout uint64 `json:"dcTimeout"`
		DcNonce   []byte `json:"dcNonce"`
		// DcExpirationTimeout blockHeight when dc bill gets removed from state tree (old spec)
		DcExpirationTimeout uint64 `json:"dcExpirationTimeout"`
		// DcExpirationTimeout block number when dc bill can be removed from state tree (system-generated txs)
		SwapTimeout uint64 `json:"swapTimeout"`

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
	return &wallet.BillProof{Bill: &wallet.Bill{
		Id:              b.GetID(),
		Value:           b.Value,
		TxHash:          b.TxHash,
		LastAddFCTxHash: b.LastAddFCTxHash,
	}, TxProof: b.TxProof}
}

// isExpired returns true if dcBill, that was left unswapped, should be deleted
func (b *Bill) isExpired(blockHeight uint64) bool {
	return b.DcNonce != nil && blockHeight >= b.DcExpirationTimeout
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
