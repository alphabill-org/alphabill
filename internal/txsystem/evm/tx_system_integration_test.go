package evm

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	evmcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

// contract Counter {
//
//	 uint256 value=0;
//
//	 event Increment(
//	     uint256 indexed newValue
//	 );
//
//	 function increment() public returns(uint256) {
//	     value++;
//	     emit Increment(value);
//	     return value;
//	 }
//
//	 function get() public view returns (uint256) {
//	    return value;
//	}
//
// }
const counterContractCode = "60806040526000805534801561001457600080fd5b506101b1806100246000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80636d4ce63c1461003b578063d09de08a14610059575b600080fd5b610043610077565b60405161005091906100e9565b60405180910390f35b610061610080565b60405161006e91906100e9565b60405180910390f35b60008054905090565b600080600081548092919061009490610133565b91905055506000547f51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a8160405160405180910390a2600054905090565b6000819050919050565b6100e3816100d0565b82525050565b60006020820190506100fe60008301846100da565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061013e826100d0565b91507fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff82036101705761016f610104565b5b60018201905091905056fea2646970667358221220e77ebad0c44e3c4060e53c55352c0cc28d52a30710a3437aa1345775714eeb1f64736f6c63430008120033"
const counterABI = "[\n\t{\n\t\t\"anonymous\": false,\n\t\t\"inputs\": [\n\t\t\t{\n\t\t\t\t\"indexed\": true,\n\t\t\t\t\"internalType\": \"uint256\",\n\t\t\t\t\"name\": \"newValue\",\n\t\t\t\t\"type\": \"uint256\"\n\t\t\t}\n\t\t],\n\t\t\"name\": \"Increment\",\n\t\t\"type\": \"event\"\n\t},\n\t{\n\t\t\"inputs\": [],\n\t\t\"name\": \"get\",\n\t\t\"outputs\": [\n\t\t\t{\n\t\t\t\t\"internalType\": \"uint256\",\n\t\t\t\t\"name\": \"\",\n\t\t\t\t\"type\": \"uint256\"\n\t\t\t}\n\t\t],\n\t\t\"stateMutability\": \"view\",\n\t\t\"type\": \"function\"\n\t},\n\t{\n\t\t\"inputs\": [],\n\t\t\"name\": \"increment\",\n\t\t\"outputs\": [\n\t\t\t{\n\t\t\t\t\"internalType\": \"uint256\",\n\t\t\t\t\"name\": \"\",\n\t\t\t\t\"type\": \"uint256\"\n\t\t\t}\n\t\t],\n\t\t\"stateMutability\": \"nonpayable\",\n\t\t\"type\": \"function\"\n\t}\n]"

var systemIdentifier = []byte{0, 0, 4, 2}

func TestEVMPartition_DeployAndCallContract(t *testing.T) {
	from := test.RandomBytes(20)
	evmPartition, err := testpartition.NewPartition(t, 3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := NewEVMTxSystem(systemIdentifier, WithInitialAddressAndBalance(from, big.NewInt(oneEth)), WithBlockDB(memorydb.New())) // 1 ETH
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)

	network, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{evmPartition})
	require.NoError(t, err)
	require.NoError(t, network.Start(t))

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
	require.Equal(t, details.ContractAddr, common.Address{})
	// expect count uint256 = 1
	count := uint256.NewInt(1)
	require.EqualValues(t, count.PaddedBytes(32), details.ReturnData)
	require.Len(t, details.Logs, 1)

	entry := details.Logs[0]
	require.Len(t, entry.Topics, 2)
	require.Equal(t, common.BytesToHash(evmcrypto.Keccak256([]byte(cABI.Events["Increment"].Sig))), entry.Topics[0])
	require.Equal(t, common.BytesToHash(count.PaddedBytes(32)), entry.Topics[1])
	require.Equal(t, contractAddr, entry.Address)
	require.Nil(t, entry.Data)
}

func createTransferTx(t *testing.T, from []byte, to []byte) *types.TransactionOrder {
	evmAttr := &TxAttributes{
		From:  from,
		To:    to,
		Value: big.NewInt(1000),
		Gas:   params.TxGas,
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
