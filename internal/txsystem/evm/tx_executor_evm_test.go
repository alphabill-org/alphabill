package evm

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
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
	_, err := execute(1, stateDB, &TxAttributes{
		From:  fromAddr.Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Gas:   10000000,
		Value: big.NewInt(0),
	}, systemIdentifier)
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
			if _, err := execute(2, stateDB, callContract, systemIdentifier); err != nil {
				b.Fatal("call transaction failed, %w", err)
			}
		}
	})
}

func initStateDBWithAccountAndSC(t *testing.T, state *rma.Tree, eoaAddr common.Address, balance uint64) *statedb.StateDB {
	t.Helper()
	stateDB := statedb.NewStateDB(state)
	stateDB.CreateAccount(eoaAddr)
	stateDB.AddBalance(eoaAddr, big.NewInt(int64(balance)))
	// create a contract
	scAddr := evmcrypto.CreateAddress(common.BytesToAddress(eoaAddr.Bytes()), 0)
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
					From: nil,
					Gas:  10,
				},
				stateDB: statedb.NewStateDB(state),
			},
			wantErrStr: "invalid evm tx, from addr is nil",
		},
		{
			name: "err - from address is account with code",
			args: args{
				attr: &TxAttributes{
					From: evmcrypto.CreateAddress(common.BytesToAddress(fromAddr.Bytes()), 0).Bytes(),
					Gas:  1000000,
				},
				stateDB: initStateDBWithAccountAndSC(t, state, fromAddr, 1000000000000),
			},
			wantErrStr: ErrSenderNotEOA.Error(),
		},
		{
			name: "err - insufficient funds",
			args: args{
				attr: &TxAttributes{
					From: fromAddr.Bytes(),
					Gas:  1000000,
				},
				stateDB: initStateDBWithAccountAndSC(t, state, fromAddr, 10),
			},
			wantErrStr: ErrInsufficientFunds.Error(),
		},
		{
			name: "ok",
			args: args{
				attr: &TxAttributes{
					From: fromAddr.Bytes(),
					Gas:  10,
				},
				stateDB: initStateDBWithAccountAndSC(t, state, fromAddr, 10000),
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
