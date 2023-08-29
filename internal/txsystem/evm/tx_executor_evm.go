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
	StateTransition struct {
		gp         *core.GasPool
		msg        *TxAttributes
		gas        uint64
		gasPrice   *big.Int
		initialGas uint64
		value      *big.Int
		data       []byte
		state      vm.StateDB
		evm        *vm.EVM
	}

	// ExecutionResult includes all output after executing given evm
	// message no matter the execution itself is successful or not.
	ExecutionResult struct {
		UsedGas    uint64 // Total used gas but include the refunded gas
		Err        error  // Any error encountered during the execution(listed in core/vm/errors.go)
		ReturnData []byte // Returned data from evm(function result or data supplied with revert opcode)
	}

	ProcessingDetails struct {
		_            struct{} `cbor:",toarray"`
		ErrorDetails string
		ReturnData   []byte
		ContractAddr common.Address
		Logs         []*statedb.LogEntry
	}
)

// Unwrap returns the internal evm error which allows us for further
// analysis outside.
func (result *ExecutionResult) Unwrap() error {
	return result.Err
}

// Failed returns the indicator whether the execution is successful or not
func (result *ExecutionResult) Failed() bool { return result.Err != nil }

// Return is a helper function to help caller distinguish between revert reason
// and function return. Return returns the data after execution if no error occurs.
func (result *ExecutionResult) Return() []byte {
	if result.Err != nil {
		return nil
	}
	return common.CopyBytes(result.ReturnData)
}

func errorToStr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
func (d *ProcessingDetails) Bytes() ([]byte, error) {
	return cbor.Marshal(d)
}

// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg *TxAttributes, gp *core.GasPool, gasUnitPrice *big.Int) *StateTransition {
	return &StateTransition{
		gp:       gp,
		evm:      evm,
		msg:      msg,
		gasPrice: gasUnitPrice,
		value:    msg.Value,
		data:     msg.Data,
		state:    evm.StateDB,
	}
}

func (st *StateTransition) buyGas() error {
	mgval := new(big.Int).SetUint64(st.msg.Gas)
	mgval = mgval.Mul(mgval, st.gasPrice)
	balanceCheck := mgval
	if have, want := st.state.GetBalance(st.msg.FromAddr()), balanceCheck; have.Cmp(want) < 0 {
		return fmt.Errorf("%w: address %v have %v want %v", errInsufficientFunds, st.msg.FromAddr().Hex(), have, want)
	}
	if err := st.gp.SubGas(st.msg.Gas); err != nil {
		return fmt.Errorf("block limit error: %w", err)
	}
	st.gas += st.msg.Gas

	st.initialGas = st.msg.Gas
	st.state.SubBalance(st.msg.FromAddr(), mgval)
	return nil
}

func (st *StateTransition) preCheck() error {
	// Make sure this transaction's nonce is correct.
	stNonce := st.state.GetNonce(st.msg.FromAddr())
	if msgNonce := st.msg.Nonce; stNonce < msgNonce {
		return fmt.Errorf("%w: address %v, tx: %d state: %d", errNonceTooHigh,
			st.msg.FromAddr().Hex(), msgNonce, stNonce)
	} else if stNonce > msgNonce {
		return fmt.Errorf("%w: address %v, tx: %d state: %d", errNonceTooLow,
			st.msg.FromAddr().Hex(), msgNonce, stNonce)
	} else if stNonce+1 < stNonce {
		return fmt.Errorf("%w: address %v, nonce: %d", errNonceMax,
			st.msg.FromAddr().Hex(), stNonce)
	}
	// Make sure the sender is an EOA
	if codeHash := st.state.GetCodeHash(st.msg.FromAddr()); codeHash != emptyCodeHash && codeHash != (common.Hash{}) {
		return fmt.Errorf("%w: address %v, codehash: %s", errSenderNotEOA,
			st.msg.FromAddr().Hex(), codeHash)
	}
	return st.buyGas()
}

func (st *StateTransition) TransitionDb() (*ExecutionResult, error) {
	if err := st.preCheck(); err != nil {
		return nil, fmt.Errorf("evm tx validation failed, %w", err)
	}
	var (
		msg              = st.msg
		sender           = vm.AccountRef(msg.FromAddr())
		rules            = st.evm.ChainConfig().Rules(st.evm.Context.BlockNumber, st.evm.Context.Random != nil)
		contractCreation = msg.To == nil
	)
	// calculate initial gas cost per tx type and input data
	gas, err := calcIntrinsicGas(st.data, contractCreation)
	if err != nil {
		return nil, fmt.Errorf("evm tx intrinsic gas calcluation failed, %w", err)
	}
	if st.gas < gas {
		return nil, fmt.Errorf("%w: address %v, tx intrinsic cost higher than max gas", errInsufficientFunds, sender.Address().Hex())
	}
	st.gas -= gas
	// if the value is not 0, then make sure caller has enough balance to cover asset transfer for **topmost** call
	if msg.Value.Sign() > 0 && !st.evm.Context.CanTransfer(st.state, msg.FromAddr(), msg.Value) {
		return nil, fmt.Errorf("%w: address %v", errInsufficientFundsForTransfer, msg.FromAddr().Hex())
	}

	// todo: investigate access lists and whether we should support them (need to be added to attributes)
	if rules.IsBerlin {
		st.state.PrepareAccessList(sender.Address(), msg.ToAddr(), vm.ActivePrecompiles(rules), ethtypes.AccessList{})
	}
	var (
		vmErr        error
		contractAddr common.Address
		ret          []byte
	)
	if contractCreation {
		// contract creation
		ret, contractAddr, st.gas, vmErr = st.evm.Create(sender, msg.Data, st.gas, msg.Value)
	} else {
		st.state.SetNonce(sender.Address(), st.state.GetNonce(sender.Address())+1)
		contractAddr = vm.AccountRef(msg.To).Address()
		ret, st.gas, vmErr = st.evm.Call(sender, contractAddr, msg.Data, st.gas, msg.Value)
	}
	// Before EIP-3529: refunds were capped to gasUsed / 2
	st.refundGas(params.RefundQuotient)
	// calculate gas price for used gas
	return &ExecutionResult{
		UsedGas:    st.gasUsed(),
		Err:        vmErr,
		ReturnData: ret,
	}, nil
}

func (st *StateTransition) refundGas(refundQuotient uint64) {
	// Apply refund counter, capped to a refund quotient
	refund := st.gasUsed() / refundQuotient
	if refund > st.state.GetRefund() {
		refund = st.state.GetRefund()
	}
	st.gas += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	st.state.AddBalance(st.msg.FromAddr(), remaining)

	// Also return remaining gas to the block gas counter, so it is
	// available for the next transaction.
	st.gp.AddGas(st.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
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
	if err := validate(attr); err != nil {
		return nil, err
	}
	blockCtx := newBlockContext(currentBlockNumber)
	evm := vm.NewEVM(blockCtx, newTxContext(attr, gasUnitPrice), stateDB, newChainConfig(new(big.Int).SetBytes(systemIdentifier)), newVMConfig())
	res, err := NewStateTransition(evm, attr, gp, gasUnitPrice).TransitionDb()
	if err != nil {
		return nil, err
	}
	// TODO: handle a case when the smart contract calls another smart contract.
	targetUnits := []types.UnitID{attr.From}
	if attr.To != nil {
		targetUnits = append(targetUnits, attr.To)
	}
	success := types.TxStatusSuccessful
	var errorDetail error
	if res.Unwrap() != nil || stateDB.DBError() != nil {
		success = types.TxStatusFailed
		if res.Unwrap() != nil {
			errorDetail = fmt.Errorf("evm runtime error: %w", res.Unwrap())
		}
		if stateDB.DBError() != nil {
			errorDetail = fmt.Errorf("%w state db error: %w", errorDetail, stateDB.DBError())
		}
	}
	// The contract address can be derived from the transaction itself
	var contractAddress common.Address
	if attr.ToAddr() == nil {
		// Deriving the signer is expensive, only do if it's actually needed
		contractAddress = ethcrypto.CreateAddress(attr.FromAddr(), attr.Nonce)
	}
	evmProcessingDetails := &ProcessingDetails{
		ReturnData:   res.ReturnData,
		ContractAddr: contractAddress,
		ErrorDetails: errorToStr(errorDetail),
	}
	if errorDetail == nil {
		evmProcessingDetails.Logs = stateDB.GetLogs()
	}
	detailBytes, err := evmProcessingDetails.Bytes()
	if err != nil {
		return nil, fmt.Errorf("evm result encode error %w", err)
	}
	txPrice := calcGasPrice(res.UsedGas, gasUnitPrice)
	log.Trace("total gas: %v gas units, price in alpha %v", res.UsedGas, weiToAlpha(txPrice))

	return &types.ServerMetadata{ActualFee: weiToAlpha(txPrice), TargetUnits: targetUnits, SuccessIndicator: success, ProcessingDetails: detailBytes}, nil
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
func validate(attr *TxAttributes) error {
	if attr.From == nil {
		return fmt.Errorf("invalid evm tx, from addr is nil")
	}
	if attr.Value == nil {
		return fmt.Errorf("invalid evm tx, value is nil")
	}
	if attr.Value.Sign() < 0 {
		return fmt.Errorf("invalid evm tx, value is negative")
	}
	return nil
}
