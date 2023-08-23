package evm

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"testing"

	abstate "github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	evmcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func BenchmarkCallContract(b *testing.B) {
	s := abstate.NewEmptyState()
	from := test.RandomBytes(20)
	stateDB := statedb.NewStateDB(s)
	fromAddr := common.BytesToAddress(from)
	stateDB.CreateAccount(fromAddr)
	stateDB.AddBalance(fromAddr, big.NewInt(1000000000000000000)) // add 1 ETH
	gasPool := new(core.GasPool).AddGas(math.MaxUint64)
	gasPrice := big.NewInt(DefaultGasPrice)
	_, err := execute(1, stateDB, &TxAttributes{
		From:  fromAddr.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Gas:   10000000,
		Value: big.NewInt(0),
	}, systemIdentifier, gasPool, gasPrice)
	require.NoError(b, err)
	scAddr := evmcrypto.CreateAddress(common.BytesToAddress(from), 0)
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(b, err)
	inc := cABI.Methods["increment"]
	callContract := &TxAttributes{
		From:  from,
		To:    scAddr.Bytes(),
		Data:  inc.ID,
		Gas:   10000000,
		Value: big.NewInt(0),
	}
	b.ResetTimer()
	b.Run("call counter contract", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = execute(2, stateDB, callContract, systemIdentifier, gasPool, gasPrice); err != nil {
				b.Fatal("call transaction failed, %w", err)
			}
		}
	})
}

func initStateDBWithAccountAndSC(t *testing.T, eoaAddr common.Address, balance uint64) *statedb.StateDB {
	t.Helper()
	s := abstate.NewEmptyState()
	stateDB := statedb.NewStateDB(s)
	stateDB.CreateAccount(eoaAddr)
	stateDB.AddBalance(eoaAddr, big.NewInt(int64(balance)))
	// deploy a contract
	evmAttr := &TxAttributes{
		From:  eoaAddr.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
		Gas:   1000000000000000,
	}
	blockCtx := newBlockContext(0)
	evm := vm.NewEVM(blockCtx, newTxContext(evmAttr, big.NewInt(0)), stateDB, newChainConfig(new(big.Int).SetBytes(systemIdentifier)), newVMConfig())
	_, _, _, err := evm.Create(vm.AccountRef(eoaAddr), evmAttr.Data, 1000000000000000, evmAttr.Value)
	require.NoError(t, err)
	require.NoError(t, stateDB.DBError())
	return stateDB
}

func Test_validate(t *testing.T) {
	s := abstate.NewEmptyState()
	fromAddr := common.BytesToAddress(test.RandomBytes(20))
	gasPrice := big.NewInt(DefaultGasPrice)
	type args struct {
		stateDB *statedb.StateDB
		attr    *TxAttributes
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "err - invalid attributes from is nil",
			args: args{
				attr: &TxAttributes{
					From:  nil,
					Value: big.NewInt(0),
					Gas:   10,
				},
				stateDB: statedb.NewStateDB(s),
			},
			wantErrStr: "invalid evm tx, from addr is nil",
		},
		{
			name: "err - value is nil",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: nil,
					Gas:   10,
				},
				stateDB: statedb.NewStateDB(s),
			},
			wantErrStr: "invalid evm tx, value is nil",
		},
		{
			name: "err - invalid negative value",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(-2),
					Gas:   0,
				},
				stateDB: statedb.NewStateDB(s),
			},
			wantErrStr: "invalid evm tx, value is negative",
		},
		{
			name: "err - from address is account with code",
			args: args{
				attr: &TxAttributes{
					From:  evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Gas:   1000000,
				},
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 1000000000000),
			},
			wantErrStr: errSenderNotEOA.Error(),
		},
		{
			name: "err - insufficient funds",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   1000000,
				},
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 10),
			},
			wantErrStr: errInsufficientFunds.Error(),
		},
		{
			name: "ok - exact amount",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   10,
				},
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 10*DefaultGasPrice),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.args.stateDB, tt.args.attr, gasPrice)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_execute(t *testing.T) {
	s := abstate.NewEmptyState()
	fromAddr := common.BytesToAddress(test.RandomBytes(20))
	gasPrice := big.NewInt(DefaultGasPrice)
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	type args struct {
		currentBlockNumber uint64
		stateDB            *statedb.StateDB
		attr               *TxAttributes
		systemIdentifier   []byte
		gp                 *core.GasPool
	}
	tests := []struct {
		name                 string
		args                 args
		wantErrStr           string
		wantSuccessIndicator types.TxStatus
		wantDetails          *ProcessingDetails
	}{
		{
			name: "err - invalid attributes from is nil",
			args: args{
				attr: &TxAttributes{
					From:  nil,
					Value: big.NewInt(0),
					Gas:   10,
				},
				stateDB: statedb.NewStateDB(s),
			},
			wantErrStr:           "invalid evm tx, from addr is nil",
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails:          nil,
		},
		{
			name: "err - insufficient funds",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   10,
				},
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 1000000),
			},
			wantErrStr:           errInsufficientFunds.Error(),
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails:          nil,
		},
		{
			name: "err - block gas limit reached",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   1000,
				},
				gp:      new(core.GasPool).AddGas(100),
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 1000*DefaultGasPrice+1),
			},
			wantErrStr:           "block limit error: gas limit reached",
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails:          nil,
		},
		{
			name: "err - not enough to pay intrinsic cost", // contract creation intrinsic cost is higher than max gas
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   1000,
				},
				gp:      new(core.GasPool).AddGas(100000),
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 1000*DefaultGasPrice+1),
			},
			wantErrStr:           "tx intrinsic cost higher than max gas",
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails:          nil,
		},
		{
			name: "err - runtime out of gas", // intrinsic cost is 0 as there is no data
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Data:  cABI.Methods["increment"].ID,
					Gas:   2300,
				},
				gp:      new(core.GasPool).AddGas(100000),
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 2500*DefaultGasPrice),
			},
			// wantErrStr - no error, add failing transaction to block, work was done and fees will be taken
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails: &ProcessingDetails{
				ErrorDetails: "evm runtime error: out of gas",
				ContractAddr: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0),
			},
		},
		{
			name: "err - not enough funds for transfer",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    test.RandomBytes(20),
					Value: big.NewInt(10),
					Gas:   0,
				},
				gp:      new(core.GasPool).AddGas(100),
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 1),
			},
			wantErrStr:           "insufficient funds for transfer",
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails:          nil,
		},
		{
			name: "err - unknown method",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Data:  make([]byte, 4),
					Gas:   53000,
				},
				gp:      new(core.GasPool).AddGas(100000),
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 53000*DefaultGasPrice+1),
			},
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails: &ProcessingDetails{
				ErrorDetails: "evm runtime error: execution reverted",
				ContractAddr: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0),
				ReturnData:   nil,
			},
		},
		{
			name: "ok - transfer to unknown recipient address",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    test.RandomBytes(20),
					Value: big.NewInt(10),
					Gas:   0,
				},
				gp:      new(core.GasPool).AddGas(100),
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 100),
			},
			wantSuccessIndicator: types.TxStatusSuccessful,
			wantDetails: &ProcessingDetails{
				ErrorDetails: "",
			},
		},
		{
			name: "ok - call get",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Data:  cABI.Methods["get"].ID,
					Gas:   53000,
				},
				gp:      new(core.GasPool).AddGas(100000),
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 53000*DefaultGasPrice+1),
			},
			wantSuccessIndicator: types.TxStatusSuccessful,
			wantDetails: &ProcessingDetails{
				ErrorDetails: "",
				ContractAddr: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0),
				ReturnData:   uint256.NewInt(0).PaddedBytes(32),
			},
		},
		{
			name: "ok - call increment method",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Data:  cABI.Methods["increment"].ID,
					Gas:   53000,
				},
				gp:      new(core.GasPool).AddGas(100000),
				stateDB: initStateDBWithAccountAndSC(t, fromAddr, 53000*DefaultGasPrice+1),
			},
			wantSuccessIndicator: types.TxStatusSuccessful,
			wantDetails: &ProcessingDetails{
				ErrorDetails: "",
				ContractAddr: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0),
				ReturnData:   uint256.NewInt(1).PaddedBytes(32),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := execute(tt.args.currentBlockNumber, tt.args.stateDB, tt.args.attr, systemIdentifier, tt.args.gp, gasPrice)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				require.Nil(t, metadata)
				return
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, metadata.SuccessIndicator, tt.wantSuccessIndicator)
			if tt.wantDetails == nil {
				require.Nil(t, metadata.ProcessingDetails)
				return
			}
			require.NotNil(t, metadata.ProcessingDetails)
			var details ProcessingDetails
			require.NoError(t, cbor.Unmarshal(metadata.ProcessingDetails, &details))
			if tt.wantDetails.ErrorDetails != "" {
				require.Equal(t, details.ErrorDetails, tt.wantDetails.ErrorDetails)
			}
			if tt.wantDetails.ReturnData != nil {
				require.Equal(t, details.ReturnData, tt.wantDetails.ReturnData)
			}
		})
	}
}

func Test_errorToStr(t *testing.T) {
	var err error = nil
	require.Equal(t, "", errorToStr(err))
	err = fmt.Errorf("custom error")
	require.Equal(t, "custom error", errorToStr(err))
}
