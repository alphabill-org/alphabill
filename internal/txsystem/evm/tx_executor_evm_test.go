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

	_, err := execute(1, stateDB, &TxAttributes{
		From:  from,
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
	}, systemIdentifier)
	require.NoError(b, err)
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(b, err)
	addr := evmcrypto.CreateAddress(common.BytesToAddress(from), 0)
	inc := cABI.Methods["increment"]
	callContract := &TxAttributes{
		From:  from,
		To:    addr.Bytes(),
		Data:  inc.ID,
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
