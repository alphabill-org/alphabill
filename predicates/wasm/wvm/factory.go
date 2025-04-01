package wvm

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

// AB "common types", ie not tx system specific stuff
type ABTypesFactory struct{}

func (ABTypesFactory) createObj(typID uint32, data []byte) (any, error) {
	var obj any
	switch typID {
	case type_id_tx_order:
		obj = &types.TransactionOrder{}
	case type_id_tx_record:
		obj = &types.TransactionRecord{}
	case type_id_tx_proof:
		obj = &types.TxProof{}
	default:
		return nil, fmt.Errorf("unknown type ID %d", typID)
	}

	if err := types.Cbor.Unmarshal(data, obj); err != nil {
		return nil, fmt.Errorf("decoding data as %T: %w", obj, err)
	}
	return obj, nil
}

const (
	type_id_tx_order  = 1
	type_id_tx_record = 2
	type_id_tx_proof  = 3
)
