package evm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/params"
)

// newChainConfig returns an Ethereum ChainConfig for EVM state transitions.
func newChainConfig(chainID *big.Int) *params.ChainConfig {
	return &params.ChainConfig{
		ChainID:             chainID,
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        big.NewInt(0),
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    big.NewInt(0),
		BerlinBlock:         big.NewInt(0), // todo: This should be disabled, no access list?
		// LondonBlock:             big.NewInt(0), no support for london system and burning of fees
		ArrowGlacierBlock:  big.NewInt(0),
		GrayGlacierBlock:   big.NewInt(0),
		MergeNetsplitBlock: big.NewInt(0),
		// Todo: enabled all new features, check what is actually needed
		ShanghaiTime: new(uint64),
		CancunTime:   new(uint64),
		PragueTime:   new(uint64),
		VerkleTime:   nil, // diabled verkel tree for now

		TerminalTotalDifficulty: nil,
		Ethash:                  nil,
		Clique:                  nil,
		IsDevMode:               false,
	}
}
