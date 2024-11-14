package wvm

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

// AB "common types", ie not tx system specific stuff
type ABTypesFactory struct{}

// must be "tx partition id + type id" to support tx system specific objects?
// or we only support generic types here?
// either the "data" (CBOR!) must have version id or version must come in as param?
// the data (CBOR) could also encode the type?
func (ABTypesFactory) createObj(typID uint32, data []byte) (any, error) {
	var obj any
	switch typID {
	case type_id_tx_order:
		obj = &types.TransactionOrder{Version: 1}
	case type_id_tx_record:
		obj = &types.TransactionRecord{Version: 1}
	case type_id_tx_proof:
		obj = &types.TxProof{Version: 1}
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
	type_id_tx_record = 8
	type_id_tx_proof  = 9
)
