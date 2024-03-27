package wvm

import (
	"fmt"

	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

// AB "common types", ie not tx system specific stuff
type ABTypesFactory struct{}

// must be "tx system id + type id"?
// or we only support generic types here?
func (ABTypesFactory) create_obj(typID uint32, data []byte) (any, error) {
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

	if err := cbor.Unmarshal(data, obj); err != nil {
		return nil, fmt.Errorf("decoding data as %T: %w", obj, err)
	}
	return obj, nil
}

const (
	type_id_tx_order  = 1
	type_id_tx_record = 8
	type_id_tx_proof  = 9
)
