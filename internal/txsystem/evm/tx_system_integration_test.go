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
	"github.com/stretchr/testify/require"
)

//	contract Counter {
//		uint256 value=0;
//		function increment() public {
//			value++;
//		}
//	}
const counterContractCode = "60806040526000805534801561001457600080fd5b5060fe806100236000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c8063d09de08a14602d575b600080fd5b60336035565b005b6000808154809291906045906085565b9190505550565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b6000819050919050565b6000608e82607b565b91507fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff820360bd5760bc604c565b5b60018201905091905056fea26469706673582212209ac42d9fb65f478cd14f35928a820d2b071fa19b593f40b088f9f2e5de0e311f64736f6c63430008110033"
const counterABI = "[\n\t{\n\t\t\"inputs\": [],\n\t\t\"name\": \"get\",\n\t\t\"outputs\": [\n\t\t\t{\n\t\t\t\t\"internalType\": \"uint256\",\n\t\t\t\t\"name\": \"\",\n\t\t\t\t\"type\": \"uint256\"\n\t\t\t}\n\t\t],\n\t\t\"stateMutability\": \"view\",\n\t\t\"type\": \"function\"\n\t},\n\t{\n\t\t\"inputs\": [],\n\t\t\"name\": \"increment\",\n\t\t\"outputs\": [],\n\t\t\"stateMutability\": \"nonpayable\",\n\t\t\"type\": \"function\"\n\t}\n]"

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
	require.Equal(t, details.VmError, "")
	require.Equal(t, details.StateError, "")
	// call contract
	contractAddr := evmcrypto.CreateAddress(common.BytesToAddress(from), 0)
	require.Equal(t, details.ContractAddr, contractAddr)
	require.NotEmpty(t, details.ReturnData) // increment does not return anything

	// call contract
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	callContractTx := createCallContractTx(from, contractAddr, cABI.Methods["increment"].ID, t)
	require.NoError(t, evmPartition.SubmitTx(callContractTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(evmPartition, callContractTx), test.WaitDuration, test.WaitTick)
	_, _, txRecord, err = evmPartition.GetTxProof(callContractTx)
	require.Equal(t, types.TxStatusSuccessful, txRecord.ServerMetadata.SuccessIndicator)
	require.NotNil(t, txRecord.ServerMetadata.ProcessingDetails)
	require.NoError(t, txRecord.UnmarshalProcessingDetails(&details))
	require.NoError(t, err)
	require.Equal(t, details.VmError, "")
	require.Equal(t, details.StateError, "")
	require.Empty(t, details.ReturnData) // increment does not return anything
	require.Equal(t, details.ContractAddr, contractAddr)
}

func createTransferTx(t *testing.T, from []byte, to []byte) *types.TransactionOrder {
	evmAttr := &TxAttributes{
		From:  from,
		To:    to,
		Value: big.NewInt(1000),
		Gas:   0, // transfer does not cost gas
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
