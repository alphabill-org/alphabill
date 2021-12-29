package wallet

import "github.com/holiman/uint256"

type bill struct {
	Id     *uint256.Int `json:"id"`
	Value  uint64       `json:"value"`
	TxHash []byte       `json:"txHash"`
}
