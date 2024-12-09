package evm

import (
	"bytes"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	evmcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
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

const networkID types.NetworkID = 5
const partitionID types.PartitionID = 0x00000402

func TestEVMPartition_DeployAndCallContract(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   networkID,
		PartitionID: 0x00000402,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   2000 * time.Millisecond,
	}
	from := test.RandomBytes(20)
	genesisState := newGenesisState(t, from, big.NewInt(oneEth))
	blockDB, err := memorydb.New()
	require.NoError(t, err)
	evmPartition, err := testpartition.NewPartition(t, 3, func(trustBase types.RootTrustBase) txsystem.TransactionSystem {
		genesisState = genesisState.Clone()
		system, err := NewEVMTxSystem(
			pdr.NetworkID,
			pdr.PartitionID,
			observability.Default(t),
			WithBlockDB(blockDB),
			WithState(genesisState),
		) // 1 ETH
		require.NoError(t, err)
		return system
	}, pdr, genesisState)
	require.NoError(t, err)

	network, err := testpartition.NewAlphabillPartition(t, []*testpartition.NodePartition{evmPartition})
	require.NoError(t, err)
	require.NoError(t, network.Start(t))
	defer network.WaitClose(t)

	// transfer
	to := test.RandomBytes(20)
	transferTx := createTransferTx(t, from, to)
	require.NoError(t, evmPartition.SubmitTx(transferTx))
	txProof, err := testpartition.WaitTxProof(t, evmPartition, transferTx)
	require.NoError(t, err, "evm transfer transaction failed")
	require.EqualValues(t, transferTx, testtransaction.FetchTxoV1(t, txProof))
	// deploy contract
	deployContractTx := createDeployContractTx(t, from)
	require.NoError(t, evmPartition.SubmitTx(deployContractTx))
	txProof, err = testpartition.WaitTxProof(t, evmPartition, deployContractTx)
	require.NoError(t, err, "evm deploy transaction failed")
	require.EqualValues(t, deployContractTx, testtransaction.FetchTxoV1(t, txProof))
	require.Equal(t, types.TxStatusSuccessful, txProof.TxRecord.ServerMetadata.SuccessIndicator)
	var details evm.ProcessingDetails
	require.NoError(t, txProof.TxRecord.UnmarshalProcessingDetails(&details))
	require.NoError(t, err)
	require.Equal(t, details.ErrorDetails, "")
	// call contract
	contractAddr := evmcrypto.CreateAddress(common.BytesToAddress(from), 1)
	require.Equal(t, details.ContractAddr, contractAddr)
	require.NotEmpty(t, details.ReturnData) // increment does not return anything

	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	// call contract - increment
	callContractTx := createCallContractTx(from, contractAddr, cABI.Methods["increment"].ID, 2, t)
	require.NoError(t, evmPartition.SubmitTx(callContractTx))
	txProof, err = testpartition.WaitTxProof(t, evmPartition, callContractTx)
	require.NoError(t, err, "evm call transaction failed")
	require.EqualValues(t, callContractTx, testtransaction.FetchTxoV1(t, txProof))
	require.Equal(t, types.TxStatusSuccessful, txProof.TxRecord.ServerMetadata.SuccessIndicator)
	require.NotNil(t, txProof.TxRecord.ServerMetadata.ProcessingDetails)
	require.NoError(t, txProof.TxRecord.UnmarshalProcessingDetails(&details))
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

func TestEVMPartition_Revert_test(t *testing.T) {
	from := test.RandomBytes(20)
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)
	blockDB, err := memorydb.New()
	require.NoError(t, err)
	genesisState := newGenesisState(t, from, big.NewInt(oneEth))
	system, err := NewEVMTxSystem(networkID, partitionID, observability.Default(t), WithBlockDB(blockDB), WithState(genesisState)) // 1 ETH
	require.NoError(t, err)

	// Simulate round 1
	require.NoError(t, system.BeginBlock(1))
	// transfer
	to := test.RandomBytes(20)
	transferTx := createTransferTx(t, from, to)
	txr, err := system.Execute(transferTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	// deploy contract
	deployContractTx := createDeployContractTx(t, from)
	txr, err = system.Execute(deployContractTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	var details evm.ProcessingDetails
	require.NoError(t, types.Cbor.Unmarshal(txr.ServerMetadata.ProcessingDetails, &details))
	require.Equal(t, details.ErrorDetails, "")
	contractAddr := evmcrypto.CreateAddress(common.BytesToAddress(from), 1)
	require.Equal(t, details.ContractAddr, contractAddr)
	require.NotEmpty(t, details.ReturnData) // increment does not return anything
	// call contract - increment
	callContractTx := createCallContractTx(from, contractAddr, cABI.Methods["increment"].ID, 2, t)
	txr, err = system.Execute(callContractTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.NoError(t, types.Cbor.Unmarshal(txr.ServerMetadata.ProcessingDetails, &details))
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
	round1EndState, err := system.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, round1EndState)
	require.NoError(t, system.Commit(&types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
		Version:      1,
		RoundNumber:  1,
		Hash:         round1EndState.Root(),
		SummaryValue: round1EndState.Summary(),
	}}))
	// Round 2, but this gets reverted
	require.NoError(t, system.BeginBlock(2))
	callContractTx = createCallContractTx(from, contractAddr, cABI.Methods["increment"].ID, 3, t)
	txr, err = system.Execute(callContractTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.NoError(t, types.Cbor.Unmarshal(txr.ServerMetadata.ProcessingDetails, &details))
	require.Equal(t, details.ErrorDetails, "")
	require.Equal(t, details.ContractAddr, common.Address{})
	count = uint256.NewInt(2)
	require.EqualValues(t, count.PaddedBytes(32), details.ReturnData)
	round2EndState, err := system.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, round2EndState)
	require.NotEqualValues(t, round1EndState.Root(), round2EndState.Root())
	// revert round 2 and do it over again
	system.Revert()
	revertedState, err := system.StateSummary()
	require.NoError(t, err)
	require.Equal(t, round1EndState.Root(), revertedState.Root())
	// Round 2 again, but this time with empty block, state should not change from round 1
	require.NoError(t, system.BeginBlock(2))
	round2EndState, err = system.EndBlock()
	require.NoError(t, err)
	require.NotEqualValues(t, round2EndState.Root(), round1EndState.Root())
}

func newGenesisState(t *testing.T, initialAccountAddress []byte, initialAccountBalance *big.Int) *state.State {
	s := state.NewEmptyState()
	if len(initialAccountAddress) > 0 && initialAccountBalance.Cmp(big.NewInt(0)) > 0 {
		address := common.BytesToAddress(initialAccountAddress)
		id := s.Savepoint()
		stateDB := statedb.NewStateDB(s, logger.New(t))
		stateDB.CreateAccount(address)
		stateDB.AddBalance(address, uint256.MustFromBig(initialAccountBalance), tracing.BalanceChangeUnspecified)
		s.ReleaseToSavepoint(id)

		_, _, err := s.CalculateRoot()
		require.NoError(t, err)
	}
	return s
}

func createTransferTx(t *testing.T, from []byte, to []byte) *types.TransactionOrder {
	evmAttr := &evm.TxAttributes{
		From:  from,
		To:    to,
		Value: big.NewInt(1000),
		Gas:   params.TxGas,
		Nonce: 0,
	}
	attrBytes, err := types.Cbor.Marshal(evmAttr)
	require.NoError(t, err)
	txo := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:      networkID,
			PartitionID:    partitionID,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			Type:           evm.TransactionTypeEVMCall,
			Attributes:     attrBytes,
			ClientMetadata: &types.ClientMetadata{Timeout: 100},
		},
	}
	// auth proof in evm is not used, however, all tx systems must have non-nil auth proof field
	require.NoError(t, txo.SetAuthProof(evm.TxAuthProof{}))
	return txo
}

func createCallContractTx(from []byte, addr common.Address, methodID []byte, nonce uint64, t *testing.T) *types.TransactionOrder {
	evmAttr := &evm.TxAttributes{
		From:  from,
		To:    addr.Bytes(),
		Data:  methodID,
		Value: big.NewInt(0),
		Gas:   100000,
		Nonce: nonce,
	}
	attrBytes, err := types.Cbor.Marshal(evmAttr)
	require.NoError(t, err)
	txo := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:      networkID,
			PartitionID:    partitionID,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			Type:           evm.TransactionTypeEVMCall,
			Attributes:     attrBytes,
			ClientMetadata: &types.ClientMetadata{Timeout: 100},
		},
	}
	// auth proof in evm is not used, however, all tx systems must have non-nil auth proof field
	require.NoError(t, txo.SetAuthProof(evm.TxAuthProof{}))
	return txo
}

func createDeployContractTx(t *testing.T, from []byte) *types.TransactionOrder {
	evmAttr := &evm.TxAttributes{
		From:  from,
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
		Gas:   1000000,
		Nonce: 1,
	}
	attrBytes, err := types.Cbor.Marshal(evmAttr)
	require.NoError(t, err)
	txo := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:      networkID,
			PartitionID:    partitionID,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			Type:           evm.TransactionTypeEVMCall,
			Attributes:     attrBytes,
			ClientMetadata: &types.ClientMetadata{Timeout: 100},
		},
	}
	// auth proof in evm is not used, however, all tx systems must have non-nil auth proof field
	require.NoError(t, txo.SetAuthProof(evm.TxAuthProof{}))
	return txo
}
