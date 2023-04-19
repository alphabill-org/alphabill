package test

import (
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

func NewUnitID(n uint64) []byte {
	return util.Uint256ToBytes(uint256.NewInt(n))
}
