package wallet

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"github.com/holiman/uint256"
)

type bill struct {
	Id     *uint256.Int `json:"id"`
	Value  uint64       `json:"value"`
	TxHash []byte       `json:"txHash"`

	// dc bill specific fields
	IsDcBill  bool                     `json:"dcBill"`
	DcTx      *transaction.Transaction `json:"dcTx"`
	DcTimeout uint64                   `json:"dcTimeout"`
	DcNonce   []byte                   `json:"dcNonce"`
}

// getId returns bill id in 32-byte big endian array
func (b *bill) getId() []byte {
	bytes32 := b.Id.Bytes32()
	return bytes32[:]
}
