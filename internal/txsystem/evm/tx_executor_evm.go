package evm

import (
	"crypto"
	"fmt"
	"math/big"
	"os"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/tracers/logger"
)

func handleEVMTx(tree *rma.Tree, algorithm crypto.Hash, systemIdentifier []byte, trustBase map[string]abcrypto.Verifier) txsystem.GenericExecuteFunc[TxAttributes] {
	return func(tx *types.TransactionOrder, attr *TxAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		from := common.BytesToAddress(attr.From)
		stateDB := statedb.NewStateDB(tree)
		if !stateDB.Exist(from) {
			return nil, fmt.Errorf(" address %v does not exist", from)
		}
		return execute(currentBlockNumber, stateDB, attr, systemIdentifier)
	}
}

func execute(currentBlockNumber uint64, stateDB *statedb.StateDB, attr *TxAttributes, systemIdentifier []byte) (*types.ServerMetadata, error) {
	blockCtx := newBlockContext(currentBlockNumber)
	evm := vm.NewEVM(blockCtx, newTxContext(attr), stateDB, newChainConfig(new(big.Int).SetBytes(systemIdentifier)), newVMConfig())
	sender := vm.AccountRef(attr.From)
	if attr.To == nil {
		// contract creation
		_, _, _, vmErr := evm.Create(sender, attr.Data, 10000000, attr.Value)
		if vmErr != nil {
			return nil, vmErr
		}
		// TODO handle gas
		// TODO handle "deploy contract" result
	} else {
		_, _, vmErr := evm.Call(sender, vm.AccountRef(attr.To).Address(), attr.Data, 100000, attr.Value)
		if vmErr != nil {
			return nil, vmErr
		}
		// TODO handle gas
		// TODO handle call result
	}
	if stateDB.DBError() != nil {
		return nil, stateDB.DBError()
	}
	return &types.ServerMetadata{}, nil
}

func newBlockContext(currentBlockNumber uint64) vm.BlockContext {
	return vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		GetHash: func(u uint64) common.Hash {
			// TODO implement after integrating a new AVLTree
			panic("get hash")
		},
		Coinbase: common.Address{},
		// TODO gas
		GasLimit:    10000000000000,
		BlockNumber: new(big.Int).SetUint64(currentBlockNumber),
		Time:        big.NewInt(1),
		Difficulty:  big.NewInt(0),
		BaseFee:     big.NewInt(0),
		Random:      nil,
	}
}

func newTxContext(attr *TxAttributes) vm.TxContext {
	return vm.TxContext{
		Origin: common.BytesToAddress(attr.From),
		// TODO gas price
		GasPrice: attr.GasPrice,
	}
}

func newVMConfig() vm.Config {
	return vm.Config{
		Debug: false,
		// TODO use AB logger
		Tracer:    logger.NewJSONLogger(nil, os.Stdout),
		NoBaseFee: true,
	}
}
