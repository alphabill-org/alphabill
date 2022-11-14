package money

import (
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

type Bill struct {
	Id     *uint256.Int          `json:"id"`
	Value  uint64                `json:"value"`
	TxHash []byte                `json:"txHash"`
	Tx     *txsystem.Transaction `json:"tx"`

	// dc bill specific fields
	IsDcBill  bool   `json:"dcBill"`
	DcTimeout uint64 `json:"dcTimeout"`
	DcNonce   []byte `json:"dcNonce"`
	// DcExpirationTimeout blockHeight when dc bill gets removed from state tree
	DcExpirationTimeout uint64 `json:"dcExpirationTimeout"`

	// block-proofs
	BlockProof *block.BlockProof `json:"blockProof"`
}

// GetId returns bill id in 32-byte big endian array
func (b *Bill) GetId() []byte {
	bytes32 := b.Id.Bytes32()
	return bytes32[:]
}

// isExpired returns true if dcBill, that was left unswapped, should be deleted
func (b *Bill) isExpired(blockHeight uint64) bool {
	return b.IsDcBill && blockHeight >= b.DcExpirationTimeout
}
