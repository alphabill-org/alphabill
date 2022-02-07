package wallet

// dcMetadata container for grouping dcMetadata by nonce
type dcMetadata struct {
	DcValueSum  uint64 `json:"dcValueSum"` // only set by wallet managed dc job
	DcTimeout   uint64 `json:"dcTimeout"`
	SwapTimeout uint64 `json:"swapTimeout"`
}

func (m *dcMetadata) isSwapRequired(blockHeight uint64, dcSum uint64) bool {
	return m.dcSumReached(dcSum) || m.timeoutReached(blockHeight)
}

func (m *dcMetadata) dcSumReached(dcSum uint64) bool {
	return m.DcValueSum > 0 && dcSum >= m.DcValueSum
}

func (m *dcMetadata) timeoutReached(blockHeight uint64) bool {
	return blockHeight == m.DcTimeout || blockHeight == m.SwapTimeout
}
