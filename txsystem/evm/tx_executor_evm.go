package evm

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"

	evmsdk "github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
)

func errorToStr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func (m *Module) executeEVMTx(_ *types.TransactionOrder, attr *evmsdk.TxAttributes, exeCtx *txsystem.TxExecutionContext) (sm *types.ServerMetadata, retErr error) {
	from := common.BytesToAddress(attr.From)
	stateDB := statedb.NewStateDB(m.options.state, m.log)
	if !stateDB.Exist(from) {
		return nil, fmt.Errorf("address %v does not exist", from)
	}
	defer func() {
		if retErr == nil {
			retErr = stateDB.Finalize()
		}
	}()
	return Execute(exeCtx.CurrentBlockNr, stateDB, m.options.blockDB, attr, m.systemIdentifier, m.blockGasCounter, m.options.gasUnitPrice, false, m.log)
}

func (m *Module) validateEVMTx(_ *types.TransactionOrder, attr *evmsdk.TxAttributes, _ *txsystem.TxExecutionContext) error {
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

func calcGasPrice(gas uint64, gasPrice *big.Int) *big.Int {
	cost := new(big.Int).SetUint64(gas)
	return cost.Mul(cost, gasPrice)
}

func Execute(currentBlockNumber uint64, stateDB *statedb.StateDB, blockDB keyvaluedb.KeyValueDB, attr *evmsdk.TxAttributes, systemIdentifier types.SystemID, gp *core.GasPool, gasUnitPrice *big.Int, fake bool, log *slog.Logger) (*types.ServerMetadata, error) {
	if err := validate(attr); err != nil {
		return nil, err
	}
	// Verify balance
	balance := stateDB.GetBalance(attr.FromAddr())
	projectedMaxFee := alphaToWei(weiToAlpha(new(big.Int).Mul(gasUnitPrice, new(big.Int).SetUint64(attr.Gas))))
	if balance.Cmp(projectedMaxFee) == -1 {
		return nil, fmt.Errorf("insufficient fee credit balance for transaction")
	}
	blockCtx := NewBlockContext(currentBlockNumber, blockDB)
	evm := vm.NewEVM(blockCtx, NewTxContext(attr, gasUnitPrice), stateDB, NewChainConfig(new(big.Int).SetBytes(systemIdentifier.Bytes())), NewVMConfig())
	msg := attr.AsMessage(gasUnitPrice, fake)
	// Apply the transaction to the current state (included in the env)
	execResult, err := core.ApplyMessage(evm, msg, gp)
	if err != nil {
		return nil, err
	}
	success := types.TxStatusSuccessful
	var errorDetail error
	if execResult.Unwrap() != nil || stateDB.DBError() != nil {
		success = types.TxStatusFailed
		if execResult.Unwrap() != nil {
			errorDetail = fmt.Errorf("evm runtime error: %w", execResult.Unwrap())
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
	evmProcessingDetails := &evmsdk.ProcessingDetails{
		ReturnData:   execResult.ReturnData,
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
	txPrice := calcGasPrice(execResult.UsedGas, gasUnitPrice)
	fee := weiToAlpha(txPrice)
	// if rounding isn't clean, add or subtract balance accordingly
	feeInWei := alphaToWei(fee)
	stateDB.AddBalance(msg.From, new(big.Int).Sub(txPrice, feeInWei))

	log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("total gas: %v gas units, price in alpha %v", execResult.UsedGas, fee), logger.Round(currentBlockNumber))
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: stateDB.GetUpdatedUnits(), SuccessIndicator: success, ProcessingDetails: detailBytes}, nil
}

func NewBlockContext(currentBlockNumber uint64, blockDB keyvaluedb.KeyValueDB) vm.BlockContext {
	return vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		GetHash: func(u uint64) common.Hash {
			// NB! SIGSEGV if blockDB is nil, this must not happen
			it := blockDB.Find(util.Uint64ToBytes(u))
			if !it.Valid() {
				return common.Hash{}
			}
			b := &types.Block{}
			if err := it.Value(b); err != nil {
				return common.Hash{}
			}
			return common.BytesToHash(b.UnicityCertificate.InputRecord.BlockHash)
		},
		Coinbase:      common.Address{},
		GasLimit:      DefaultBlockGasLimit,
		BlockNumber:   new(big.Int).SetUint64(currentBlockNumber),
		Time:          1,
		Difficulty:    big.NewInt(0),
		BaseFee:       big.NewInt(0),
		Random:        nil,
		ExcessBlobGas: nil,
	}
}

func NewTxContext(attr *evmsdk.TxAttributes, gasPrice *big.Int) vm.TxContext {
	return vm.TxContext{
		Origin:   common.BytesToAddress(attr.From),
		GasPrice: gasPrice,
	}
}

func NewVMConfig() vm.Config {
	return vm.Config{
		// TODO use AB logger
		Tracer:                  nil, // logger.NewJSONLogger(nil, os.Stdout),
		NoBaseFee:               true,
		EnablePreimageRecording: false, // Enables recording of SHA3/keccak preimages
	}
}

// validate - validate EVM call attributes
func validate(attr *evmsdk.TxAttributes) error {
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
