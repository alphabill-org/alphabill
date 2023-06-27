package evm

import (
	"crypto"
	"errors"
	"fmt"
	"math/big"
	"os"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/tracers/logger"
	"github.com/ethereum/go-ethereum/params"
)

const (
	txGasContractCreation uint64 = 53000 // Per transaction that creates a contract
)

var (
	emptyCodeHash = ethcrypto.Keccak256Hash(nil)
	// todo: initial constant, needs fine tuning
	gasUnitPrice = big.NewInt(210000000)

	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrSenderNotEOA      = errors.New("sender not an eoa")
	ErrGasOverflow       = errors.New("gas uint64 overflow")
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

func calcGasPrice(gas uint64) *big.Int {
	cost := new(big.Int).SetUint64(gas)
	return cost.Mul(cost, gasUnitPrice)
}

func execute(currentBlockNumber uint64, stateDB *statedb.StateDB, attr *TxAttributes, systemIdentifier []byte) (*types.ServerMetadata, error) {
	if err := validate(stateDB, attr); err != nil {
		return nil, fmt.Errorf("evm tx validation failed, %w", err)
	}
	var (
		sender             = vm.AccountRef(attr.From)
		gasRemaining       = attr.Gas
		isContractCreation = attr.To == nil
	)
	// todo: "gas handling": Subtract the max gas cost from callers balance, will be refunded later in case less is used
	// stateDB.SubBalance(sender.Address(), calcGasPrice(attr.Gas))
	// calculate initial gas cost per tx type and input data
	gas, err := calcIntrinsicGas(attr.Data, isContractCreation)
	if err != nil {
		return nil, fmt.Errorf("evm tx intrinsic gas calcluation failed, %w", err)
	}
	if gasRemaining < gas {
		return nil, fmt.Errorf("%w: address %v", ErrInsufficientFunds, sender.Address().Hex())
	}
	gasRemaining -= gas
	blockCtx := newBlockContext(currentBlockNumber)
	evm := vm.NewEVM(blockCtx, newTxContext(attr), stateDB, newChainConfig(new(big.Int).SetBytes(systemIdentifier)), newVMConfig())
	var vmErr error
	if isContractCreation {
		// contract creation
		_, _, gasRemaining, vmErr = evm.Create(sender, attr.Data, gasRemaining, attr.Value)
		// TODO handle "deploy contract" result
	} else {
		// TODO set nonce
		_, gasRemaining, vmErr = evm.Call(sender, vm.AccountRef(attr.To).Address(), attr.Data, gasRemaining, attr.Value)
		// TODO handle call result
	}
	// todo: "gas handling" Refund ETH for remaining gas, exchanged at the original rate.
	// stateDB.AddBalance(sender.Address(), calcGasPrice(gasRemaining))
	if vmErr != nil {
		return nil, vmErr
	}
	if stateDB.DBError() != nil {
		return nil, stateDB.DBError()
	}
	// todo: "gas handling" currently failing transactions are not added to block, hence we can only charge for successful calls
	// calculate gas price for used gas
	txPrice := calcGasPrice(attr.Gas - gasRemaining)
	log.Trace("total gas: %v gas units", attr.Gas-gasRemaining)
	log.Trace("total tx cost: %v mia", weiToAlpha(txPrice))
	stateDB.SubBalance(sender.Address(), txPrice)

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
		// TODO: using hardcoded gas price, should come from system description record instead?
		GasPrice: gasUnitPrice,
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

// IntrinsicGas computes the 'intrinsic gas' for an evm call with the given data.
func calcIntrinsicGas(data []byte, isContractCreation bool) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if isContractCreation {
		gas = txGasContractCreation
	}
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		nonZeroGas := params.TxDataNonZeroGasFrontier
		if (math.MaxUint64-gas)/nonZeroGas < nz {
			return 0, ErrGasOverflow
		}
		gas += nz * nonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			return 0, ErrGasOverflow
		}
		gas += z * params.TxDataZeroGas
	}
	return gas, nil
}

// validate - validate EVM call attributes
func validate(stateDB *statedb.StateDB, attr *TxAttributes) error {
	if attr.From == nil {
		return fmt.Errorf("invalid evm tx, from addr is nil")
	}
	from := vm.AccountRef(attr.From)
	// todo: add nonce/backlink to attributes and validate here

	// Make sure that calling account is not smart contract, user call cannot be a smart contract account
	if codeHash := stateDB.GetCodeHash(from.Address()); codeHash != emptyCodeHash && codeHash != (common.Hash{}) {
		return fmt.Errorf("%w: address %v, codehash: %s", ErrSenderNotEOA,
			from.Address().Hex(), codeHash)
	}
	// Verify enough funds to run
	// If the sender account does not have enough to pay for max gas, then do not execute
	if have, want := stateDB.GetBalance(from.Address()), calcGasPrice(attr.Gas); have.Cmp(want) < 0 {
		return fmt.Errorf("%w: address %v have %v want %v", ErrInsufficientFunds, from.Address().Hex(), have, want)
	}
	return nil
}
