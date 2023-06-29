package evm

import (
	"bytes"
	"math"
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	evmcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func BenchmarkCallContract(b *testing.B) {
	state := rma.NewWithSHA256()
	from := test.RandomBytes(20)
	stateDB := statedb.NewStateDB(state)
	fromAddr := common.BytesToAddress(from)
	stateDB.CreateAccount(fromAddr)
	stateDB.AddBalance(fromAddr, big.NewInt(1000000000000000000)) // add 1 ETH
	gasPool := new(core.GasPool).AddGas(1000000000000000000)
	_, err := execute(1, stateDB, &TxAttributes{
		From:  fromAddr.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Gas:   10000000,
		Value: big.NewInt(0),
		Nonce: 0,
	}, systemIdentifier, gasPool)
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
			if _, err := execute(2, stateDB, callContract, systemIdentifier, gasPool); err != nil {
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
}

func initStateDBWithAccountAndSC(t *testing.T, eoa *testAccount) *statedb.StateDB {
	t.Helper()
	state := rma.NewWithSHA256()
	stateDB := statedb.NewStateDB(state)
	stateDB.CreateAccount(eoa.Addr)
	stateDB.AddBalance(eoa.Addr, big.NewInt(int64(eoa.Balance)))
	stateDB.SetNonce(eoa.Addr, eoa.Nonce)
	// create a contract
	scAddr := evmcrypto.CreateAddress(common.BytesToAddress(eoa.Addr.Bytes()), 0)
	stateDB.CreateAccount(scAddr)
	stateDB.SetCode(scAddr, common.Hex2Bytes(counterContractCode))
	return stateDB
}

func Test_validate(t *testing.T) {
	state := rma.NewWithSHA256()
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
				stateDB: statedb.NewStateDB(state),
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
				stateDB: statedb.NewStateDB(state),
			},
			wantErrStr: "invalid evm tx, value is nil",
		},
		{
			name: "err - from address is account with code",
			args: args{
				attr: &TxAttributes{
					From:  evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Gas:   1000000,
					Nonce: 0,
				},
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 1000000000000}),
			},
			wantErrStr: ErrSenderNotEOA.Error(),
		},
		{
			name: "err - insufficient funds",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   1000000,
					Nonce: 0,
				},
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 10}),
			},
			wantErrStr: ErrInsufficientFunds.Error(),
		},
		{
			name: "err - nonce too high",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   10,
					Nonce: 1,
				},
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 10 * defaultGasPrice}),
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
					Nonce: 1,
				},
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 10 * defaultGasPrice, Nonce: 2}),
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
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 10 * defaultGasPrice, Nonce: math.MaxUint64}),
			},
			wantErrStr: "nonce has max value",
		},
		{
			name: "ok - exact amount",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   10,
					Nonce: 0,
				},
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 10 * defaultGasPrice}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.args.stateDB, tt.args.attr)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_execute(t *testing.T) {
	state := rma.NewWithSHA256()
	fromAddr := common.BytesToAddress(test.RandomBytes(20))

	type args struct {
		currentBlockNumber uint64
		stateDB            *statedb.StateDB
		attr               *TxAttributes
		systemIdentifier   []byte
		gp                 *core.GasPool
	}
	tests := []struct {
		name       string
		args       args
		wantMeta   *types.ServerMetadata
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
				stateDB: statedb.NewStateDB(state),
			},
			wantErrStr: "invalid evm tx, from addr is nil",
		},
		{
			name: "err - insufficient funds",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					Value: big.NewInt(0),
					Gas:   10,
				},
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 1000000}),
			},
			wantErrStr: ErrInsufficientFunds.Error(),
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
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 1000*defaultGasPrice + 1}),
			},
			wantErrStr: "block limit error: gas limit reached",
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
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 1000*defaultGasPrice + 1}),
			},
			wantErrStr: "tx intrinsic cost higher than max gas",
		},
		{
			name: "err - runtime out of gas", // intrinsic cost is 0 as there is no data
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Gas:   2300,
				},
				gp:      new(core.GasPool).AddGas(100000),
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 2500 * defaultGasPrice}),
			},
			wantErrStr: "evm runtime error: out of gas",
		},
		{
			name: "ok",
			args: args{
				attr: &TxAttributes{
					From:  fromAddr.Bytes(),
					To:    evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Value: big.NewInt(0),
					Gas:   53000,
				},
				gp:      new(core.GasPool).AddGas(100000),
				stateDB: initStateDBWithAccountAndSC(t, &testAccount{Addr: fromAddr, Balance: 53000*defaultGasPrice + 1}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := execute(tt.args.currentBlockNumber, tt.args.stateDB, tt.args.attr, tt.args.systemIdentifier, tt.args.gp)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				require.Nil(t, metadata)
				return
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_ReplayContractCreation(t *testing.T) {
	state := rma.NewWithSHA256()
	from := test.RandomBytes(20)
	stateDB := statedb.NewStateDB(state)
	fromAddr := common.BytesToAddress(from)
	stateDB.CreateAccount(fromAddr)
	stateDB.AddBalance(fromAddr, big.NewInt(1000000000000000000)) // add 1 ETH
	gasPool := new(core.GasPool).AddGas(blockGasLimit)
	createCounterAttr := &TxAttributes{
		From:  fromAddr.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Gas:   10000000,
		Value: big.NewInt(0),
		Nonce: 0}
	_, err := execute(1, stateDB, createCounterAttr, systemIdentifier, gasPool)
	require.NoError(t, err)
	// Try to replay
	_, err = execute(1, stateDB, createCounterAttr, systemIdentifier, gasPool)
	require.ErrorContains(t, err, "evm tx validation failed, nonce too low")
}

func Test_ReplayCall(t *testing.T) {
	state := rma.NewWithSHA256()
	from := test.RandomBytes(20)
	stateDB := statedb.NewStateDB(state)
	fromAddr := common.BytesToAddress(from)
	stateDB.CreateAccount(fromAddr)
	stateDB.AddBalance(fromAddr, big.NewInt(1000000000000000000)) // add 1 ETH
	gasPool := new(core.GasPool).AddGas(blockGasLimit)
	createCounterAttr := &TxAttributes{
		From:  fromAddr.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Gas:   10000000,
		Value: big.NewInt(0),
		Nonce: 0}
	_, err := execute(1, stateDB, createCounterAttr, systemIdentifier, gasPool)
	require.NoError(t, err)
	scAddr := evmcrypto.CreateAddress(common.BytesToAddress(from), 0)
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	inc := cABI.Methods["increment"]
	callContract := &TxAttributes{
		From:  from,
		To:    scAddr.Bytes(),
		Data:  inc.ID,
		Gas:   10000000,
		Value: big.NewInt(0),
		Nonce: 1,
	}
	_, err = execute(2, stateDB, callContract, systemIdentifier, gasPool)
	require.NoError(t, err)
	// try to replay
	_, err = execute(2, stateDB, callContract, systemIdentifier, gasPool)
	require.ErrorContains(t, err, "evm tx validation failed, nonce too low")
}
