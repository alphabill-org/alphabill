package evm

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/ethereum/go-ethereum/core/tracing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	abstate "github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	evmcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const oneEth = 1000000000000000000

func BenchmarkCallContract(b *testing.B) {
	log := logger.NOP()
	s := abstate.NewEmptyState()
	from := test.RandomBytes(20)
	stateDB := statedb.NewStateDB(s, log)
	fromAddr := common.BytesToAddress(from)
	stateDB.CreateAccount(fromAddr)
	stateDB.AddBalance(fromAddr, uint256.NewInt(oneEth), tracing.BalanceChangeUnspecified) // add 1 ETH
	gasPool := new(core.GasPool).AddGas(math.MaxUint64)
	gasPrice := big.NewInt(DefaultGasPrice)
	blockDB, err := memorydb.New()
	require.NoError(b, err)

	_, err = Execute(1, stateDB, blockDB, &evm.TxAttributes{
		From:  fromAddr.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Gas:   10000000,
		Value: big.NewInt(0),
	}, nil, systemIdentifier, gasPool, gasPrice, false, log)
	require.NoError(b, err)
	scAddr := evmcrypto.CreateAddress(common.BytesToAddress(from), 0)
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(b, err)
	inc := cABI.Methods["increment"]
	callContract := &evm.TxAttributes{
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
			blockDB, _ := memorydb.New()
			if _, err = Execute(2, stateDB, blockDB, callContract, nil, systemIdentifier, gasPool, gasPrice, false, log); err != nil {
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
	stateDB := statedb.NewStateDB(s, logger.New(t))
	for _, eoa := range accounts {
		stateDB.CreateAccount(eoa.Addr)
		stateDB.AddBalance(eoa.Addr, uint256.NewInt(eoa.Balance), tracing.BalanceChangeUnspecified)
		if len(eoa.Code) != 0 {
			// deploy a contract
			evmAttr := &evm.TxAttributes{
				From:  eoa.Addr.Bytes(),
				Data:  common.Hex2Bytes(eoa.Code),
				Value: big.NewInt(0),
				Gas:   1000000000000000,
				Nonce: 0,
			}
			blockDB, err := memorydb.New()
			require.NoError(t, err)
			blockCtx := NewBlockContext(0, blockDB)
			evm := vm.NewEVM(blockCtx, NewTxContext(evmAttr, big.NewInt(0)), stateDB, NewChainConfig(new(big.Int).SetBytes(systemIdentifier.Bytes())), NewVMConfig())
			_, _, _, err = evm.Create(vm.AccountRef(eoa.Addr), evmAttr.Data, 1000000000000000, uint256.MustFromBig(evmAttr.Value))
			require.NoError(t, err)
			if eoa.Nonce != 0 {
				stateDB.SetNonce(eoa.Addr, eoa.Nonce)
			}
		}
	}

	require.NoError(t, stateDB.DBError())
	require.NoError(t, stateDB.Finalize())
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(&types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{Version: 1,
		RoundNumber:  1,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}))
	return stateDB
}

func Test_validate(t *testing.T) {
	fromAddr := common.BytesToAddress(test.RandomBytes(20))
	type args struct {
		attr *evm.TxAttributes
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "err - invalid attributes from is nil",
			args: args{
				attr: &evm.TxAttributes{
					From:  nil,
					Value: big.NewInt(0),
					Gas:   10,
					Nonce: 0,
				},
			},
			wantErrStr: "invalid evm transaction, from addr is nil",
		},
		{
			name: "err - value is nil",
			args: args{
				attr: &evm.TxAttributes{
					From:  fromAddr.Bytes(),
					Value: nil,
					Gas:   10,
					Nonce: 0,
				},
			},
			wantErrStr: "invalid evm transaction, value is nil",
		},
		{
			name: "err - invalid negative value",
			args: args{
				attr: &evm.TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(-2),
					Gas:   0,
				},
			},
			wantErrStr: "invalid evm transaction, value is negative",
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
		attr               *evm.TxAttributes
		gp                 *core.GasPool
	}
	tests := []struct {
		name                 string
		args                 args
		wantErrStr           string
		wantSuccessIndicator types.TxStatus
		wantDetails          *evm.ProcessingDetails
		wantUpdatedUnits     int
	}{
		{
			name: "err - nonce too high",
			args: args{
				attr: &evm.TxAttributes{
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
				attr: &evm.TxAttributes{
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
				attr: &evm.TxAttributes{
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
			name: "err - from address is account with code",
			args: args{
				attr: &evm.TxAttributes{
					From:  evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Gas:   1000000,
					Nonce: 2,
				},
				gp: new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{
					{Addr: fromAddr, Balance: oneEth, Code: counterContractCode},
					{Addr: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0), Balance: oneEth, Code: counterContractCode}}),
			},
			wantErrStr: core.ErrSenderNoEOA.Error(),
		},
		{
			name: "err - insufficient funds",
			args: args{
				attr: &evm.TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(1),
					Gas:   10,
					Data:  common.Hex2Bytes(counterContractCode),
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 10 * DefaultGasPrice, Code: counterContractCode}}),
			},
			wantErrStr: core.ErrInsufficientFunds.Error(),
		},
		{
			name: "err - insufficient fee balance",
			args: args{
				attr: &evm.TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(10),
					Gas:   1000000,
					Data:  common.Hex2Bytes(counterContractCode),
					Nonce: 1,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 10, Code: counterContractCode}}),
			},
			wantErrStr: "insufficient fee credit balance for transaction",
		},
		{
			name: "err - insufficient funds to execute call get",
			args: args{
				attr: &evm.TxAttributes{
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
			wantDetails: &evm.ProcessingDetails{
				ErrorDetails: "evm runtime error: out of gas",
				ContractAddr: common.Address{},
			},
			wantUpdatedUnits: 1, // caller still gets charged since work is done
		},
		{
			name: "err - block gas limit reached",
			args: args{
				attr: &evm.TxAttributes{
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
				attr: &evm.TxAttributes{
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
				attr: &evm.TxAttributes{
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
			wantDetails: &evm.ProcessingDetails{
				ErrorDetails: "evm runtime error: out of gas",
				ContractAddr: common.Address{},
			},
			wantUpdatedUnits: 1, // caller still gets charged since work is done
		},
		{
			name: "err - not enough funds for transfer",
			args: args{
				attr: &evm.TxAttributes{
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
				attr: &evm.TxAttributes{
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
			wantDetails: &evm.ProcessingDetails{
				ErrorDetails: "evm runtime error: execution reverted",
				ContractAddr: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0),
				ReturnData:   nil,
			},
			wantUpdatedUnits: 1, // caller still gets charged since work is done
		},
		{
			name: "ok - transfer exact amount",
			args: args{
				attr: &evm.TxAttributes{
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
			wantDetails: &evm.ProcessingDetails{
				ErrorDetails: "",
				ContractAddr: common.Address{},
			},
			wantUpdatedUnits: 2, // caller still gets charged since work is done
		},
		{
			name: "ok - transfer to unknown recipient address",
			args: args{
				attr: &evm.TxAttributes{
					From:  fromAddr.Bytes(),
					To:    common.FromHex("0x4a18f39d69cb1b2f7278345df2ba4d691470e908"),
					Value: big.NewInt(10),
					Gas:   params.TxGas, // default cost per transaction no creating a contract
					Nonce: 0,
				},
				gp:      new(core.GasPool).AddGas(DefaultBlockGasLimit),
				stateDB: initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: params.TxGas*DefaultGasPrice + 10}}),
			},
			wantSuccessIndicator: types.TxStatusSuccessful,
			wantDetails: &evm.ProcessingDetails{
				ErrorDetails: "",
			},
			wantUpdatedUnits: 2, // caller still gets charged since work is done
		},
		{
			name: "ok - call get",
			args: args{
				attr: &evm.TxAttributes{
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
			wantDetails: &evm.ProcessingDetails{
				ErrorDetails: "",
				ContractAddr: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0),
				ReturnData:   uint256.NewInt(0).PaddedBytes(32),
			},
			wantUpdatedUnits: 1, // only eor gets credited for the read call
		},
		{
			name: "ok - call increment method",
			args: args{
				attr: &evm.TxAttributes{
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
			wantDetails: &evm.ProcessingDetails{
				ErrorDetails: "",
				ContractAddr: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0),
				ReturnData:   uint256.NewInt(1).PaddedBytes(32),
			},
			wantUpdatedUnits: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockDB, err := memorydb.New()
			require.NoError(t, err)
			metadata, err := Execute(tt.args.currentBlockNumber, tt.args.stateDB, blockDB, tt.args.attr, nil, systemIdentifier, tt.args.gp, gasPrice, false, logger.New(t))
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
			require.Len(t, metadata.TargetUnits, tt.wantUpdatedUnits)
			require.NotNil(t, metadata.ProcessingDetails)
			var details evm.ProcessingDetails
			require.NoError(t, types.Cbor.Unmarshal(metadata.ProcessingDetails, &details))
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
	log := logger.New(t)
	gasPrice := big.NewInt(DefaultGasPrice)
	gasPool := new(core.GasPool).AddGas(DefaultBlockGasLimit)
	eoaAddr := common.BytesToAddress(test.RandomBytes(20))
	s := abstate.NewEmptyState()
	stateDB := statedb.NewStateDB(s, log)
	stateDB.CreateAccount(eoaAddr)
	initialBalance := uint256.NewInt(1000000000000000000)
	stateDB.AddBalance(eoaAddr, initialBalance, tracing.BalanceChangeUnspecified) // add 1 ETH
	// deploy a contract
	evmAttr := &evm.TxAttributes{
		From:  eoaAddr.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
		Gas:   100000,
		Nonce: 0,
	}
	blockDB, err := memorydb.New()
	require.NoError(t, err)
	metadata, err := Execute(1, stateDB, blockDB, evmAttr, nil, systemIdentifier, gasPool, gasPrice, false, log)
	require.NoError(t, err)
	require.NotNil(t, metadata)
	// check that fee and account balance add up to initial value
	require.EqualValues(t, initialBalance, new(uint256.Int).Add(alphaToWei(metadata.ActualFee), stateDB.GetBalance(eoaAddr)))
	// Try to replay
	_, err = Execute(1, stateDB, blockDB, evmAttr, nil, systemIdentifier, gasPool, gasPrice, false, log)
	require.ErrorContains(t, err, "nonce too low")
}

func Test_ReplayCall(t *testing.T) {
	fromAddr := common.BytesToAddress(test.RandomBytes(20))
	gasPrice := big.NewInt(DefaultGasPrice)
	stateDB := initStateDBWithAccountAndSC(t, []*testAccount{{Addr: fromAddr, Balance: 2 * (53000 * DefaultGasPrice), Code: counterContractCode}})
	gasPool := new(core.GasPool).AddGas(DefaultBlockGasLimit)
	scAddr := evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0)
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)
	inc := cABI.Methods["increment"]
	callContract := &evm.TxAttributes{
		From:  fromAddr.Bytes(),
		To:    scAddr.Bytes(),
		Data:  inc.ID,
		Gas:   53000,
		Value: big.NewInt(0),
		Nonce: 1,
	}
	blockDB, err := memorydb.New()
	require.NoError(t, err)
	log := logger.New(t)
	metadata, err := Execute(2, stateDB, blockDB, callContract, nil, systemIdentifier, gasPool, gasPrice, false, log)
	require.NoError(t, err)
	require.NotNil(t, metadata)
	// check that fee and account balance add up to initial value
	initialBalance := uint256.NewInt(2 * (53000 * DefaultGasPrice)) // this is the value set as balance in initStateDBWithAccountAndSC
	require.EqualValues(t, initialBalance, new(uint256.Int).Add(alphaToWei(metadata.ActualFee), stateDB.GetBalance(fromAddr)))

	// try to replay
	_, err = Execute(2, stateDB, blockDB, callContract, nil, systemIdentifier, gasPool, gasPrice, false, log)
	require.ErrorContains(t, err, "nonce too low")
}

// smart contract that returns previous block hash when calling method previousBlockHash()
const getPreviousHashCode = "608060405234801561001057600080fd5b5061012b806100206000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c8063257aeacc14602d575b600080fd5b60336047565b604051603e91906076565b60405180910390f35b6000806001436055919060c8565b9050804091505090565b6000819050919050565b607081605f565b82525050565b6000602082019050608960008301846069565b92915050565b6000819050919050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600060d182608f565b915060da83608f565b925082820390508181111560ef5760ee6099565b5b9291505056fea2646970667358221220003ccde3343fa4317032d2145e484a26296debb62e8d436908101235d9a621cf64736f6c63430008120033"
const getPreviousHashABI = "[\n\t{\n\t\t\"inputs\": [],\n\t\t\"name\": \"previousBlockHash\",\n\t\t\"outputs\": [\n\t\t\t{\n\t\t\t\t\"internalType\": \"bytes32\",\n\t\t\t\t\"name\": \"\",\n\t\t\t\t\"type\": \"bytes32\"\n\t\t\t}\n\t\t],\n\t\t\"stateMutability\": \"view\",\n\t\t\"type\": \"function\"\n\t}\n]"

func Test_PreviousBlockHashFunction(t *testing.T) {
	log := logger.New(t)
	s := abstate.NewEmptyState()
	from := test.RandomBytes(20)
	stateDB := statedb.NewStateDB(s, log)
	fromAddr := common.BytesToAddress(from)
	stateDB.CreateAccount(fromAddr)
	stateDB.AddBalance(fromAddr, uint256.NewInt(oneEth), tracing.BalanceChangeUnspecified) // add 1 ETH
	gasPool := new(core.GasPool).AddGas(math.MaxUint64)
	gasPrice := big.NewInt(DefaultGasPrice)
	mockDB, err := memorydb.New()
	require.NoError(t, err)
	b := &types.Block{
		Header:             &types.Header{SystemID: evm.DefaultSystemID},
		Transactions:       []*types.TransactionRecord{},
		UnicityCertificate: &types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{Version: 1, RoundNumber: 1, BlockHash: test.RandomBytes(32)}},
	}
	require.NoError(t, mockDB.Write(util.Uint64ToBytes(uint64(1)), &b))
	_, err = Execute(2, stateDB, mockDB, &evm.TxAttributes{
		From:  fromAddr.Bytes(),
		Data:  common.Hex2Bytes(getPreviousHashCode),
		Gas:   10000000,
		Value: big.NewInt(0),
	}, nil, systemIdentifier, gasPool, gasPrice, false, log)
	require.NoError(t, err)
	scAddr := evmcrypto.CreateAddress(common.BytesToAddress(from), 0)
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(getPreviousHashABI)))
	require.NoError(t, err)
	inc := cABI.Methods["previousBlockHash"]
	callContract := &evm.TxAttributes{
		From:  from,
		To:    scAddr.Bytes(),
		Data:  inc.ID,
		Gas:   10000000,
		Value: big.NewInt(0),
		Nonce: 1,
	}
	res, err := Execute(2, stateDB, mockDB, callContract, nil, systemIdentifier, gasPool, gasPrice, false, log)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, types.TxStatusSuccessful, res.SuccessIndicator)
	var details evm.ProcessingDetails
	require.NoError(t, types.Cbor.Unmarshal(res.ProcessingDetails, &details))
	require.EqualValues(t, b.UnicityCertificate.InputRecord.BlockHash, details.ReturnData)
	// query not existing block
	callContract.Nonce++
	res, err = Execute(3, stateDB, mockDB, callContract, nil, systemIdentifier, gasPool, gasPrice, false, log)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, types.TxStatusSuccessful, res.SuccessIndicator)
	require.NoError(t, types.Cbor.Unmarshal(res.ProcessingDetails, &details))
	// expect 0H to be returned
	require.EqualValues(t, make([]byte, 32), details.ReturnData)
}

func Test_errorToStr(t *testing.T) {
	var err error = nil
	require.Equal(t, "", errorToStr(err))
	err = fmt.Errorf("custom error")
	require.Equal(t, "custom error", errorToStr(err))
}
