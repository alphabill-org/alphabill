package unit

import (
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/ethereum/go-ethereum/common"
)

const (
	UnitIDLength   = UnitPartLength + TypePartLength
	UnitPartLength = 20
	TypePartLength = 1
)

var (
	EvmAccountType = []byte{0x01}
)

// NewFeeCreditRecordID - evm accounts act as FCR as well
// EVM cannot be sharded hence shard part is ignored
func NewFeeCreditRecordID(_ []byte, unitPart []byte) types.UnitID {
	u, _ := NewEvmAccountID(unitPart)
	return u
}

// NewEvmAccountID - creates new EVM account unit. All EVM native types are of EvmAccountType
// For EVM account acts both as an account and also as fee credit record (FCR)
func NewEvmAccountID(unitPart []byte) (types.UnitID, error) {
	if len(unitPart) != UnitPartLength {
		return nil, fmt.Errorf("invalid account address length, must be 20 bytes")
	}
	// NB! currently evm cannot be sharded
	return types.NewUnitID(UnitIDLength, nil, unitPart, EvmAccountType), nil
}

// NewEvmAccountIDFromAddress - creates new EVM account unit. All EVM native types are of EvmAccountType
// For EVM account acts both as an account and also as fee credit record (FCR)
func NewEvmAccountIDFromAddress(unitPart common.Address) types.UnitID {
	// NB! currently evm cannot be sharded
	return types.NewUnitID(UnitIDLength, nil, unitPart[:], EvmAccountType)
}

// AddressFromUnitID - extract ethereum address from unit ID
func AddressFromUnitID(id types.UnitID) common.Address {
	// NB! currently evm cannot be sharded
	return common.BytesToAddress(id[:UnitIDLength-TypePartLength])
}

func NewUnitData(unitID types.UnitID) (state.UnitData, error) {
	if unitID.HasType(EvmAccountType) {
		return &StateObject{}, nil
	}
	return nil, fmt.Errorf("unknown unit type in UnitID %s", unitID)
}
