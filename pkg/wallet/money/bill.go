package money

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/types"
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
		// DcExpirationTimeout blockHeight when dc bill gets removed from state tree
		DcExpirationTimeout uint64 `json:"dcExpirationTimeout"`

		// fcb specific fields
		// AddFCTxHash last add fee credit tx hash
		AddFCTxHash []byte `json:"addFcTxHash,omitempty"`
	}
)

// GetID returns bill id in 32-byte big endian array
func (b *Bill) GetID() []byte {
	if b != nil {
		return util.Uint256ToBytes(b.Id)
	}
	return nil
}

func (b *Bill) ToGenericBill() *wallet.Bill {
	return &wallet.Bill{
		Id:          b.GetID(),
		Value:       b.Value,
		TxHash:      b.TxHash,
		TxProof:     b.TxProof,
		AddFCTxHash: b.AddFCTxHash,
	}
}

// isExpired returns true if dcBill, that was left unswapped, should be deleted
func (b *Bill) isExpired(blockHeight uint64) bool {
	return b.DcNonce != nil && blockHeight >= b.DcExpirationTimeout
}

func (b *Bill) addProof(txIdx int, bl *types.Block) error {
	proof, err := wallet.NewTxProof(txIdx, bl, crypto.SHA256)
	if err != nil {
		return err
	}
	b.TxProof = proof
	return nil
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

func (b *Bill) GetAddFCTxHash() []byte {
	if b != nil {
		return b.AddFCTxHash
	}
	return nil
}
