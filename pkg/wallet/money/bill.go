package money

import (
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

type bill struct {
	Id     *uint256.Int `json:"id"`
	Value  uint64       `json:"value"`
	TxHash []byte       `json:"txHash"`

	// dc bill specific fields
	IsDcBill  bool                  `json:"dcBill"`
	DcTx      *txsystem.Transaction `json:"dcTx"`
	DcTimeout uint64                `json:"dcTimeout"`
	DcNonce   []byte                `json:"dcNonce"`
	// DcExpirationTimeout blockHeight when dc bill gets removed from state
	DcExpirationTimeout uint64 `json:"dcExpirationTimeout"`
}

// getId returns bill id in 32-byte big endian array
func (b *bill) getId() []byte {
	bytes32 := b.Id.Bytes32()
	return bytes32[:]
}

// isExpired returns true if dcBill, that was left unswapped, should be deleted
func (b *bill) isExpired(blockHeight uint64) bool {
	return b.IsDcBill && blockHeight >= b.DcExpirationTimeout
}
