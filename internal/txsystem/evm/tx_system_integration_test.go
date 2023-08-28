package evm

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	evmcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

//	contract Counter {
//	 uint256 count = 0;
//
//	 function increment() public returns (uint256) {
//	   count++;
//	 }
//
//	 function get() public view returns (uint256){
//	   return count;
//	 }
//	}
const counterContractCode = "60806040526000805534801561001457600080fd5b50610182806100246000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80636d4ce63c1461003b578063d09de08a14610059575b600080fd5b610043610077565b60405161005091906100ba565b60405180910390f35b610061610080565b60405161006e91906100ba565b60405180910390f35b60008054905090565b600080600081548092919061009490610104565b9190505550600054905090565b6000819050919050565b6100b4816100a1565b82525050565b60006020820190506100cf60008301846100ab565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061010f826100a1565b91507fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff8203610141576101406100d5565b5b60018201905091905056fea2646970667358221220b29225b3d241fa8f111f1815e66aa98f82e8014e2ee748995fbc05f8852cf07c64736f6c63430008120033"
const counterABI = "[{\"inputs\":[],\"name\":\"get\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"increment\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]"

var systemIdentifier = []byte{0, 0, 4, 2}

func TestEVMPartition_DeployAndCallContract(t *testing.T) {
	from := test.RandomBytes(20)
	evmPartition, err := testpartition.NewPartition(3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := NewEVMTxSystem(systemIdentifier, WithInitialAddressAndBalance(from, big.NewInt(1000000000000000000))) // 1 ETH
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)

	network, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{evmPartition})
	require.NoError(t, err)
	require.NoError(t, network.Start())

	// transfer
	to := test.RandomBytes(20)
	transferTx := createTransferTx(t, from, to)
	require.NoError(t, evmPartition.SubmitTx(transferTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(evmPartition, transferTx), test.WaitDuration, test.WaitTick)

	// deploy contract
	deployContractTx := createDeployContractTx(t, from)
	require.NoError(t, evmPartition.SubmitTx(deployContractTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(evmPartition, deployContractTx), test.WaitDuration, test.WaitTick)
	_, _, txRecord, err := evmPartition.GetTxProof(deployContractTx)
	require.Equal(t, types.TxStatusSuccessful, txRecord.ServerMetadata.SuccessIndicator)
	var details ProcessingDetails
	require.NoError(t, txRecord.UnmarshalProcessingDetails(&details))
	require.NoError(t, err)
	require.Equal(t, details.ErrorDetails, "")
	// call contract
	contractAddr := evmcrypto.CreateAddress(common.BytesToAddress(from), 1)
	require.Equal(t, details.ContractAddr, contractAddr)
	require.NotEmpty(t, details.ReturnData) // increment does not return anything

	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	// call contract - increment
	callContractTx := createCallContractTx(from, contractAddr, cABI.Methods["increment"].ID, t)
	require.NoError(t, evmPartition.SubmitTx(callContractTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(evmPartition, callContractTx), test.WaitDuration, test.WaitTick)
	_, _, txRecord, err = evmPartition.GetTxProof(callContractTx)
	require.Equal(t, types.TxStatusSuccessful, txRecord.ServerMetadata.SuccessIndicator)
	require.NotNil(t, txRecord.ServerMetadata.ProcessingDetails)
	require.NoError(t, txRecord.UnmarshalProcessingDetails(&details))
	require.NoError(t, err)
	require.Equal(t, details.ErrorDetails, "")
	require.Equal(t, details.ContractAddr, contractAddr)
	// expect count uint256 = 1
	count := uint256.NewInt(1)
	require.EqualValues(t, count.PaddedBytes(32), details.ReturnData)
}

func createTransferTx(t *testing.T, from []byte, to []byte) *types.TransactionOrder {
	evmAttr := &TxAttributes{
		From:  from,
		To:    to,
		Value: big.NewInt(1000),
		Gas:   0, // transfer does not cost gas
		Nonce: 0,
	}
	attrBytes, err := cbor.Marshal(evmAttr)
	require.NoError(t, err)
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           PayloadTypeEVMCall,
			SystemID:       systemIdentifier,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			ClientMetadata: &types.ClientMetadata{Timeout: 100},
			Attributes:     attrBytes,
		},
		OwnerProof: nil,
	}
}

func createCallContractTx(from []byte, addr common.Address, methodID []byte, t *testing.T) *types.TransactionOrder {
	evmAttr := &TxAttributes{
		From:  from,
		To:    addr.Bytes(),
		Data:  methodID,
		Value: big.NewInt(0),
		Gas:   100000,
		Nonce: 2,
	}
	attrBytes, err := cbor.Marshal(evmAttr)
	require.NoError(t, err)
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           PayloadTypeEVMCall,
			SystemID:       systemIdentifier,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			ClientMetadata: &types.ClientMetadata{Timeout: 100},
			Attributes:     attrBytes,
		},
		OwnerProof: nil,
	}
}

func createDeployContractTx(t *testing.T, from []byte) *types.TransactionOrder {
	evmAttr := &TxAttributes{
		From:  from,
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
		Gas:   1000000,
		Nonce: 1,
	}
	attrBytes, err := cbor.Marshal(evmAttr)
	require.NoError(t, err)
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           PayloadTypeEVMCall,
			SystemID:       systemIdentifier,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			ClientMetadata: &types.ClientMetadata{Timeout: 100},
			Attributes:     attrBytes,
		},
		OwnerProof: nil,
	}
}
