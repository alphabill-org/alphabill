package evm

import (
	"errors"
	"fmt"
	"math/big"
	"os"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/tracers/logger"
	"github.com/ethereum/go-ethereum/params"
	"github.com/fxamacker/cbor/v2"
)

const (
	// todo: initial constants, need fine-tuning
	txGasContractCreation uint64 = 53000 // Per transaction that creates a contract
)

var (
	emptyCodeHash = ethcrypto.Keccak256Hash(nil)

	errInsufficientFunds            = errors.New("insufficient funds")
	errInsufficientFundsForTransfer = errors.New("insufficient funds for transfer")
	errSenderNotEOA                 = errors.New("sender not an eoa")
	errGasOverflow                  = errors.New("gas uint64 overflow")
	errNonceTooLow                  = errors.New("nonce too low")
	errNonceTooHigh                 = errors.New("nonce too high")
	errNonceMax                     = errors.New("nonce has max value")
)

type (
	ProcessingDetails struct {
		_            struct{} `cbor:",toarray"`
		ErrorDetails string
		ReturnData   []byte
		ContractAddr common.Address
		Logs         []*statedb.LogEntry
	}
)

func (d *ProcessingDetails) Bytes() ([]byte, error) {
	return cbor.Marshal(d)
}

func errorToStr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func handleEVMTx(systemIdentifier []byte, opts *Options, blockGas *core.GasPool) txsystem.GenericExecuteFunc[TxAttributes] {
	return func(tx *types.TransactionOrder, attr *TxAttributes, currentBlockNumber uint64) (sm *types.ServerMetadata, err error) {
		from := common.BytesToAddress(attr.From)
		stateDB := statedb.NewStateDB(opts.state)
		if !stateDB.Exist(from) {
			return nil, fmt.Errorf(" address %v does not exist", from)
		}
		defer func() {
			if err == nil {
				err = stateDB.Finalize()
			}
		}()
		return execute(currentBlockNumber, stateDB, attr, systemIdentifier, blockGas, opts.gasUnitPrice)
	}
}

func calcGasPrice(gas uint64, gasPrice *big.Int) *big.Int {
	cost := new(big.Int).SetUint64(gas)
	return cost.Mul(cost, gasPrice)
}

func execute(currentBlockNumber uint64, stateDB *statedb.StateDB, attr *TxAttributes, systemIdentifier []byte, gp *core.GasPool, gasUnitPrice *big.Int) (*types.ServerMetadata, error) {
	if err := validate(stateDB, attr, gasUnitPrice); err != nil {
		return nil, fmt.Errorf("evm tx validation failed, %w", err)
	}
	if err := gp.SubGas(attr.Gas); err != nil {
		return nil, fmt.Errorf("block limit error: %w", err)
	}
	var (
		sender             = vm.AccountRef(attr.FromAddr())
		gasRemaining       = attr.Gas
		isContractCreation = attr.To == nil
		toAddr             = attr.ToAddr()
	)
	// Subtract the max gas cost from callers balance, will be refunded later in case less is used
	stateDB.SubBalance(sender.Address(), calcGasPrice(attr.Gas, gasUnitPrice))
	// calculate initial gas cost per tx type and input data
	gas, err := calcIntrinsicGas(attr.Data, isContractCreation)
	if err != nil {
		return nil, fmt.Errorf("evm tx intrinsic gas calcluation failed, %w", err)
	}
	if gasRemaining < gas {
		return nil, fmt.Errorf("%w: address %v, tx intrinsic cost higher than max gas", errInsufficientFunds, sender.Address().Hex())
	}
	gasRemaining -= gas
	blockCtx := newBlockContext(currentBlockNumber)
	evm := vm.NewEVM(blockCtx, newTxContext(attr, gasUnitPrice), stateDB, newChainConfig(new(big.Int).SetBytes(systemIdentifier)), newVMConfig())
	rules := evm.ChainConfig().Rules(evm.Context.BlockNumber, evm.Context.Random != nil)
	// if the value is not 0, then make sure caller has enough balance to cover asset transfer for **topmost** call
	if attr.Value.Sign() > 0 && !evm.Context.CanTransfer(stateDB, attr.FromAddr(), attr.Value) {
		return nil, fmt.Errorf("%w: address %v", errInsufficientFundsForTransfer, attr.FromAddr().Hex())
	}
	// todo: investigate access lists and whether we should support them (need to be added to attributes)
	if rules.IsBerlin {
		stateDB.PrepareAccessList(sender.Address(), toAddr, vm.ActivePrecompiles(rules), ethtypes.AccessList{})
	}
	var (
		vmErr        error
		contractAddr common.Address
		ret          []byte
	)
	if isContractCreation {
		// contract creation
		ret, contractAddr, gasRemaining, vmErr = evm.Create(sender, attr.Data, gasRemaining, attr.Value)
	} else {
		stateDB.SetNonce(sender.Address(), stateDB.GetNonce(sender.Address())+1)
		contractAddr = vm.AccountRef(attr.To).Address()
		ret, gasRemaining, vmErr = evm.Call(sender, contractAddr, attr.Data, gasRemaining, attr.Value)
	}
	// Refund ETH for remaining gas, exchanged at the original rate.
	stateDB.AddBalance(sender.Address(), calcGasPrice(gasRemaining, gasUnitPrice))
	// calculate gas price for used gas
	txPrice := calcGasPrice(attr.Gas-gasRemaining, gasUnitPrice)
	log.Trace("total gas: %v gas units, price in alpha %v", attr.Gas-gasRemaining, weiToAlpha(txPrice))
	// also add remaining back to block gas pool
	gp.AddGas(gasRemaining)
	// TODO: handle a case when the smart contract calls another smart contract.
	targetUnits := []types.UnitID{attr.From}
	if attr.To != nil {
		targetUnits = append(targetUnits, attr.To)
	}
	success := types.TxStatusSuccessful
	var errorDetail error
	if vmErr != nil || stateDB.DBError() != nil {
		success = types.TxStatusFailed
		if vmErr != nil {
			errorDetail = fmt.Errorf("evm runtime error: %w", vmErr)
		}
		if stateDB.DBError() != nil {
			errorDetail = fmt.Errorf("%w state db error: %w", errorDetail, stateDB.DBError())
		}
	}
	evmProcessingDetails := &ProcessingDetails{
		ReturnData:   ret,
		ContractAddr: contractAddr,
		ErrorDetails: errorToStr(errorDetail),
	}
	if errorDetail == nil {
		evmProcessingDetails.Logs = stateDB.GetLogs()
	}
	detailBytes, err := evmProcessingDetails.Bytes()
	if err != nil {
		return nil, fmt.Errorf("evm result encode error %w", err)
	}
	return &types.ServerMetadata{TargetUnits: targetUnits, SuccessIndicator: success, ProcessingDetails: detailBytes}, nil
}

func newBlockContext(currentBlockNumber uint64) vm.BlockContext {
	return vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		GetHash: func(u uint64) common.Hash {
			// TODO implement after integrating a new AVLTree
			panic("get hash")
		},
		Coinbase:    common.Address{},
		GasLimit:    DefaultBlockGasLimit,
		BlockNumber: new(big.Int).SetUint64(currentBlockNumber),
		Time:        big.NewInt(1),
		Difficulty:  big.NewInt(0),
		BaseFee:     big.NewInt(0),
		Random:      nil,
	}
}

func newTxContext(attr *TxAttributes, gasPrice *big.Int) vm.TxContext {
	return vm.TxContext{
		Origin:   common.BytesToAddress(attr.From),
		GasPrice: gasPrice,
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
			return 0, errGasOverflow
		}
		gas += nz * nonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			return 0, errGasOverflow
		}
		gas += z * params.TxDataZeroGas
	}
	return gas, nil
}

// validate - validate EVM call attributes
func validate(stateDB *statedb.StateDB, attr *TxAttributes, gasPrice *big.Int) error {
	if attr.From == nil {
		return fmt.Errorf("invalid evm tx, from addr is nil")
	}
	if attr.Value == nil {
		return fmt.Errorf("invalid evm tx, value is nil")
	}
	if attr.Value.Sign() < 0 {
		return fmt.Errorf("invalid evm tx, value is negative")
	}
	from := vm.AccountRef(attr.From)
	// Make sure this transaction's nonce is correct.
	stNonce := stateDB.GetNonce(attr.FromAddr())
	if msgNonce := attr.Nonce; stNonce < msgNonce {
		return fmt.Errorf("%w: address %v, tx: %d state: %d", errNonceTooHigh,
			attr.FromAddr().Hex(), msgNonce, stNonce)
	} else if stNonce > msgNonce {
		return fmt.Errorf("%w: address %v, tx: %d state: %d", errNonceTooLow,
			attr.FromAddr().Hex(), msgNonce, stNonce)
	} else if stNonce+1 < stNonce {
		return fmt.Errorf("%w: address %v, nonce: %d", errNonceMax,
			attr.FromAddr().Hex(), stNonce)
	}
	// Make sure that calling account is not smart contract, user call cannot be a smart contract account
	if codeHash := stateDB.GetCodeHash(from.Address()); codeHash != emptyCodeHash && codeHash != (common.Hash{}) {
		return fmt.Errorf("%w: address %v, codehash: %s", errSenderNotEOA,
			from.Address().Hex(), codeHash)
	}
	// Verify enough funds to run
	// If the sender account does not have enough to pay for max gas, then do not execute
	if have, want := stateDB.GetBalance(from.Address()), calcGasPrice(attr.Gas, gasPrice); have.Cmp(want) < 0 {
		return fmt.Errorf("%w: address %v have %v want %v", errInsufficientFunds, from.Address().Hex(), have, want)
	}
	return nil
}
