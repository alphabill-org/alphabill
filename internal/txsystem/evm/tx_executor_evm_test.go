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
	"github.com/ethereum/go-ethereum/params"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const oneEth = 1000000000000000000

func BenchmarkCallContract(b *testing.B) {
	s := abstate.NewEmptyState()
	from := test.RandomBytes(20)
	stateDB := statedb.NewStateDB(s)
	fromAddr := common.BytesToAddress(from)
	stateDB.CreateAccount(fromAddr)
	stateDB.AddBalance(fromAddr, big.NewInt(oneEth)) // add 1 ETH
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
		Nonce: 1,
	}
	b.ResetTimer()
	b.Run("call counter contract", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = execute(2, stateDB, callContract, systemIdentifier, gasPool, gasPrice); err != nil {
				b.Fatal("call transaction failed, %w", err)
			}
			callContract.Nonce += 1
		}
	})
}

type testAccount struct {
	Addr    common.Address
	Balance uint64
	Nonce   uint64
	Code    string
}

func initStateDBWithAccountAndSC(t *testing.T, accounts []*testAccount) *statedb.StateDB {
	t.Helper()
	s := abstate.NewEmptyState()
	stateDB := statedb.NewStateDB(s)
	for _, eoa := range accounts {
		stateDB.CreateAccount(eoa.Addr)
		stateDB.AddBalance(eoa.Addr, big.NewInt(int64(eoa.Balance)))
		if len(eoa.Code) != 0 {
			// deploy a contract
			evmAttr := &TxAttributes{
				From:  eoa.Addr.Bytes(),
				Data:  common.Hex2Bytes(eoa.Code),
				Value: big.NewInt(0),
				Gas:   1000000000000000,
				Nonce: 0,
			}
			blockCtx := newBlockContext(0)
			evm := vm.NewEVM(blockCtx, newTxContext(evmAttr, big.NewInt(0)), stateDB, newChainConfig(new(big.Int).SetBytes(systemIdentifier)), newVMConfig())
			_, _, _, err := evm.Create(vm.AccountRef(eoa.Addr), evmAttr.Data, 1000000000000000, evmAttr.Value)
			require.NoError(t, err)
			if eoa.Nonce != 0 {
				stateDB.SetNonce(eoa.Addr, eoa.Nonce)
			}
		}
	}

	require.NoError(t, stateDB.DBError())
	return stateDB
}

func Test_validate(t *testing.T) {
	fromAddr := common.BytesToAddress(test.RandomBytes(20))
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
					Nonce: 0,
				},
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
					Nonce: 0,
				},
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
			},
			wantErrStr: "invalid evm tx, value is negative",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.args.attr)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_execute(t *testing.T) {
	fromAddr := common.BytesToAddress(test.RandomBytes(20))
	toAddr := common.BytesToAddress(test.RandomBytes(20))
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
			name: "err - from address is account with code",
			args: args{
				attr: &TxAttributes{
					From:  evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Gas:   1000000,
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: oneEth, Code: counterContractCode}}),
			},
			wantErrStr: core.ErrSenderNoEOA.Error(),
		},
		{
			name: "err - nonce too high",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   10,
					Nonce: 2,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 10 * DefaultGasPrice, Code: counterContractCode}}),
			},
			wantErrStr: "nonce too high",
		},
		{
			name: "err - nonce too low",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   10,
					Nonce: 0,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 10 * DefaultGasPrice, Nonce: 2, Code: counterContractCode}}),
			},
			wantErrStr: "nonce too low",
		},
		{
			name: "err - nonce overflow",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   10,
					Nonce: math.MaxUint64,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 10 * DefaultGasPrice, Nonce: math.MaxUint64, Code: counterContractCode}}),
			},
			wantErrStr: "nonce has max value",
		},
		{
			name: "err - insufficient funds",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   1000000,
					Data:  common.Hex2Bytes(counterContractCode),
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 10, Code: counterContractCode}}),
			},
			wantErrStr: core.ErrInsufficientFunds.Error(),
		},
		{
			name: "err - insufficient funds to execute call get",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Data:  cABI.Methods["get"].ID,
					Gas:   params.TxGas + (uint64(len(cABI.Methods["get"].ID)) * params.TxDataNonZeroGasEIP2028), // default cost + 16 * nof data bytes, is enough for intrinsic cost but not execution
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: oneEth, Code: counterContractCode}}),
			},
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails: &ProcessingDetails{
				ErrorDetails: "evm runtime error: out of gas",
				ContractAddr: common.Address{},
			},
		},
		{
			name: "err - block gas limit reached",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   1000,
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(100),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 1000*DefaultGasPrice + 1, Code: counterContractCode}}),
			},
			wantErrStr:           "gas limit reached",
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
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(100000),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 1000*DefaultGasPrice + 1, Code: counterContractCode}}),
			},
			wantErrStr:           "intrinsic gas too low: have 1000, want 53000",
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails:          nil,
		},
		{
			name: "err - runtime out of gas",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Data:  cABI.Methods["get"].ID,
					Gas:   params.TxGas + (uint64(len(cABI.Methods["get"].ID)) * params.TxDataNonZeroGasEIP2028), // default cost + 16 * nof data bytes, is enough for intrinsic cost but not execution
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: oneEth, Code: counterContractCode}}),
			},
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails: &ProcessingDetails{
				ErrorDetails: "evm runtime error: out of gas",
				ContractAddr: common.Address{},
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
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(100),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 1, Code: counterContractCode}}),
			},
			wantErrStr:           "insufficient funds for gas * price + value",
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
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(100000),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 53000*DefaultGasPrice + 1, Code: counterContractCode}}),
			},
			wantSuccessIndicator: types.TxStatusFailed,
			wantDetails: &ProcessingDetails{
				ErrorDetails: "evm runtime error: execution reverted",
				ContractAddr: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0),
				ReturnData:   nil,
			},
		},
		{
			name: "ok - transfer exact amount",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    toAddr.Bytes(),
					Value: big.NewInt(10),
					Gas:   params.TxGas, // default cost per transaction no creating a contract
					Nonce: 0,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: (params.TxGas + 10) * DefaultGasPrice}, {Addr: toAddr}}),
			},
			wantSuccessIndicator: types.TxStatusSuccessful,
			wantDetails: &ProcessingDetails{
				ErrorDetails: "",
				ContractAddr: common.Address{},
			},
		},
		{
			name: "ok - transfer to unknown recipient address",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    test.RandomBytes(20),
					Value: big.NewInt(10),
					Gas:   params.TxGas, // default cost per transaction no creating a contract
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: params.TxGas*DefaultGasPrice + 10, Code: counterContractCode}}),
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
					Gas:   params.TxGas + (uint64(len(cABI.Methods["get"].ID)) * params.TxDataNonZeroGasEIP2028) + 10000, // default cost + 16 * nof data bytes + some for execution
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: oneEth, Code: counterContractCode}}),
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
					Gas:   50000,
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: oneEth, Code: counterContractCode}}),
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
			require.Equal(t, tt.wantSuccessIndicator, metadata.SuccessIndicator)
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

func Test_ReplayContractCreation(t *testing.T) {
	gasPrice := big.NewInt(DefaultGasPrice)
	gasPool := new(core.GasPool).AddGas(DefaultBlockGasLimit)
	eoaAddr := common.BytesToAddress(test.RandomBytes(20))
	s := abstate.NewEmptyState()
	stateDB := statedb.NewStateDB(s)
	stateDB.CreateAccount(eoaAddr)
	stateDB.AddBalance(eoaAddr, big.NewInt(1000000000000000000)) // add 1 ETH
	// deploy a contract
	evmAttr := &TxAttributes{
		From:  eoaAddr.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
		Gas:   100000,
		Nonce: 0,
	}
	_, err := execute(1, stateDB, evmAttr, systemIdentifier, gasPool, gasPrice)
	require.NoError(t, err)
	// Try to replay
	_, err = execute(1, stateDB, evmAttr, systemIdentifier, gasPool, gasPrice)
	require.ErrorContains(t, err, "nonce too low")
}

func Test_ReplayCall(t *testing.T) {
	fromAddr := common.BytesToAddress(test.RandomBytes(20))
	gasPrice := big.NewInt(DefaultGasPrice)
	stateDB := initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 53000 * DefaultGasPrice, Code: counterContractCode}})
	gasPool := new(core.GasPool).AddGas(DefaultBlockGasLimit)
	scAddr := evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0)
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	inc := cABI.Methods["increment"]
	callContract := &TxAttributes{
		From:  fromAddr.Bytes(),
		To:    scAddr.Bytes(),
		Data:  inc.ID,
		Gas:   53000,
		Value: big.NewInt(0),
		Nonce: 1,
	}
	_, err = execute(2, stateDB, callContract, systemIdentifier, gasPool, gasPrice)
	require.NoError(t, err)
	// try to replay
	_, err = execute(2, stateDB, callContract, systemIdentifier, gasPool, gasPrice)
	require.ErrorContains(t, err, "nonce too low")
}

func Test_errorToStr(t *testing.T) {
	var err error = nil
	require.Equal(t, "", errorToStr(err))
	err = fmt.Errorf("custom error")
	require.Equal(t, "custom error", errorToStr(err))
}
