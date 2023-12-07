package alphabill

import (
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

func NewTransaction(tx *types.TransactionOrder) (*Transaction, error) {
	txBytes, err := cbor.Marshal(tx)
	if err != nil {
		return nil, err
	}
	return &Transaction{Order: txBytes}, nil
}
