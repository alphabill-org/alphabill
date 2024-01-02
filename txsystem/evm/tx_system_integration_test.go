package evm

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/partition"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/unit"
	fcunit "github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/types"
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
	from := common.BytesToAddress(test.RandomBytes(20))
	fcrID := unit.NewFeeCreditRecordID(nil, from.Bytes())
	genesisState := newGenesisStateWithFC(t, fcrID, oneAlpha)
	evmPartition, err := testpartition.NewPartition(t, 3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		genesisState = genesisState.Clone()
		system, err := NewEVMTxSystem(
			systemIdentifier,
			logger.New(t),
			WithTrustBase(trustBase),
			WithBlockDB(memorydb.New()),
			WithState(genesisState),
		) // 1 ETH
		require.NoError(t, err)
		return system
	}, systemIdentifier, genesisState)
	require.NoError(t, err)
	network, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{evmPartition})
	require.NoError(t, err)
	require.NoError(t, network.Start(t))
	defer network.WaitClose(t)

	// transfer
	to := test.RandomBytes(20)
	transferTx := createTransferTx(t, from, to)
	require.NoError(t, evmPartition.SubmitTx(transferTx))
	txRecord, _, err := testpartition.WaitTxProof(t, evmPartition, transferTx)
	require.NoError(t, err, "evm transfer tx failed")
	require.EqualValues(t, transferTx, txRecord.TransactionOrder)
	// deploy contract
	deployContractTx := createDeployContractTx(t, fcrID)
	require.NoError(t, evmPartition.SubmitTx(deployContractTx))
	txRecord, _, err = testpartition.WaitTxProof(t, evmPartition, deployContractTx)
	require.NoError(t, err, "evm deploy tx failed")
	require.EqualValues(t, deployContractTx, txRecord.TransactionOrder)
	require.Equal(t, types.TxStatusSuccessful, txRecord.ServerMetadata.SuccessIndicator)
	var details ProcessingDetails
	require.NoError(t, txRecord.UnmarshalProcessingDetails(&details))
	require.NoError(t, err)
	require.Equal(t, details.ErrorDetails, "")
	// call contract
	scID := unit.NewEvmAccountIDFromAddress(evmcrypto.CreateAddress(from, 1))
	require.Equal(t, details.ContractUnitID, scID)
	require.NotEmpty(t, details.ReturnData) // increment does not return anything

	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)

	// call contract - increment
	callContractTx := createCallContractTx(scID, fcrID, cABI.Methods["increment"].ID, 2, t)
	require.NoError(t, evmPartition.SubmitTx(callContractTx))
	txRecord, _, err = testpartition.WaitTxProof(t, evmPartition, callContractTx)
	require.NoError(t, err, "evm call tx failed")
	require.EqualValues(t, callContractTx, txRecord.TransactionOrder)
	require.Equal(t, types.TxStatusSuccessful, txRecord.ServerMetadata.SuccessIndicator)
	require.NotNil(t, txRecord.ServerMetadata.ProcessingDetails)
	require.NoError(t, txRecord.UnmarshalProcessingDetails(&details))
	require.NoError(t, err)
	require.Equal(t, details.ErrorDetails, "")
	require.Empty(t, details.ContractUnitID)
	// expect count uint256 = 1
	count := uint256.NewInt(1)
	require.EqualValues(t, count.PaddedBytes(32), details.ReturnData)
	require.Len(t, details.Logs, 1)

	entry := details.Logs[0]
	require.Len(t, entry.Topics, 2)
	require.Equal(t, common.BytesToHash(evmcrypto.Keccak256([]byte(cABI.Events["Increment"].Sig))), entry.Topics[0])
	require.Equal(t, common.BytesToHash(count.PaddedBytes(32)), entry.Topics[1])
	require.Equal(t, unit.AddressFromUnitID(scID), entry.Address)
	require.Nil(t, entry.Data)
}

func TestEVMPartition_Revert_test(t *testing.T) {
	from := common.BytesToAddress(test.RandomBytes(20))
	fcrID := unit.NewFeeCreditRecordID(nil, from.Bytes())
	_, v := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v}
	cABI, err := abi.JSON(bytes.NewBuffer([]byte(counterABI)))
	require.NoError(t, err)
	genesisState := newGenesisStateWithFC(t, fcrID, oneAlpha)
	system, err := NewEVMTxSystem(systemIdentifier, logger.New(t), WithTrustBase(rootTrust), WithBlockDB(memorydb.New()), WithState(genesisState)) // 1 ETH
	require.NoError(t, err)

	// Simulate round 1
	require.NoError(t, system.BeginBlock(1))
	// transfer
	to := test.RandomBytes(20)
	transferTx := createTransferTx(t, from, to)
	meta, err := system.Execute(transferTx)
	require.NoError(t, err)
	require.NotNil(t, meta)
	// deploy contract
	deployContractTx := createDeployContractTx(t, fcrID)
	meta, err = system.Execute(deployContractTx)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, types.TxStatusSuccessful, meta.SuccessIndicator)
	var details ProcessingDetails
	require.NoError(t, cbor.Unmarshal(meta.ProcessingDetails, &details))
	require.Equal(t, details.ErrorDetails, "")
	scID := unit.NewEvmAccountIDFromAddress(evmcrypto.CreateAddress(from, 1))
	require.Equal(t, details.ContractUnitID, scID)
	require.NotEmpty(t, details.ReturnData) // increment does not return anything
	// call contract - increment
	callContractTx := createCallContractTx(scID, fcrID, cABI.Methods["increment"].ID, 2, t)
	meta, err = system.Execute(callContractTx)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, types.TxStatusSuccessful, meta.SuccessIndicator)
	require.NoError(t, cbor.Unmarshal(meta.ProcessingDetails, &details))
	require.Equal(t, details.ErrorDetails, "")
	require.Empty(t, details.ContractUnitID)
	// expect count uint256 = 1
	count := uint256.NewInt(1)
	require.EqualValues(t, count.PaddedBytes(32), details.ReturnData)
	require.Len(t, details.Logs, 1)
	entry := details.Logs[0]
	require.Len(t, entry.Topics, 2)
	require.Equal(t, common.BytesToHash(evmcrypto.Keccak256([]byte(cABI.Events["Increment"].Sig))), entry.Topics[0])
	require.Equal(t, common.BytesToHash(count.PaddedBytes(32)), entry.Topics[1])
	require.Equal(t, unit.AddressFromUnitID(scID), entry.Address)
	require.Nil(t, entry.Data)
	round1EndState, err := system.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, round1EndState)
	require.NoError(t, system.Commit(&types.UnicityCertificate{InputRecord: &types.InputRecord{
		RoundNumber:  1,
		Hash:         round1EndState.Root(),
		SummaryValue: round1EndState.Summary(),
	}}))
	// Round 2, but this gets reverted
	require.NoError(t, system.BeginBlock(2))
	callContractTx = createCallContractTx(scID, fcrID, cABI.Methods["increment"].ID, 3, t)
	meta, err = system.Execute(callContractTx)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, types.TxStatusSuccessful, meta.SuccessIndicator)
	require.NoError(t, cbor.Unmarshal(meta.ProcessingDetails, &details))
	require.Equal(t, details.ErrorDetails, "")
	require.Empty(t, details.ContractUnitID)
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

func newGenesisStateWithFC(t *testing.T, feeCreditID types.UnitID, amount uint64) *state.State {
	s := state.NewEmptyState()
	require.NoError(t, s.Apply(
		fcunit.AddCredit(feeCreditID, templates.AlwaysTrueBytes(), unit.NewEvmFcr(amount, make([]byte, 32), 1000)),
	))
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	return s
}

func createTransferTx(t *testing.T, from common.Address, to []byte) *types.TransactionOrder {
	evmAttr := &TxAttributes{
		From:  from.Bytes(),
		To:    to,
		Value: big.NewInt(1000),
		Gas:   params.TxGas,
		Nonce: 0,
	}
	attrBytes, err := cbor.Marshal(evmAttr)
	require.NoError(t, err)
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:     PayloadTypeEVMCall,
			SystemID: systemIdentifier,
			UnitID:   hash.Sum256(test.RandomBytes(32)),
			ClientMetadata: &types.ClientMetadata{
				FeeCreditRecordID: unit.NewEvmAccountIDFromAddress(from),
				Timeout:           100,
				MaxTransactionFee: 2,
			},
			Attributes: attrBytes,
		},
		OwnerProof: nil,
	}
}

func createCallContractTx(scID types.UnitID, fcrID types.UnitID, methodID []byte, nonce uint64, t *testing.T) *types.TransactionOrder {
	evmAttr := &TxAttributes{
		From:  unit.AddressFromUnitID(fcrID).Bytes(),
		To:    unit.AddressFromUnitID(scID).Bytes(),
		Data:  methodID,
		Value: big.NewInt(0),
		Gas:   100000,
		Nonce: nonce,
	}
	attrBytes, err := cbor.Marshal(evmAttr)
	require.NoError(t, err)
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:     PayloadTypeEVMCall,
			SystemID: systemIdentifier,
			UnitID:   scID,
			ClientMetadata: &types.ClientMetadata{
				FeeCreditRecordID: fcrID,
				Timeout:           100,
				MaxTransactionFee: 2,
			},
			Attributes: attrBytes,
		},
		OwnerProof: nil,
	}
}

func createDeployContractTx(t *testing.T, fcrID types.UnitID) *types.TransactionOrder {
	evmAttr := &TxAttributes{
		From:  unit.AddressFromUnitID(fcrID).Bytes(),
		Data:  common.Hex2Bytes(counterContractCode),
		Value: big.NewInt(0),
		Gas:   1000000,
		Nonce: 1,
	}
	attrBytes, err := cbor.Marshal(evmAttr)
	require.NoError(t, err)
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:     PayloadTypeEVMCall,
			SystemID: systemIdentifier,
			UnitID:   fcrID,
			ClientMetadata: &types.ClientMetadata{
				FeeCreditRecordID: fcrID,
				Timeout:           100,
				MaxTransactionFee: 2,
			},
			Attributes: attrBytes,
		},
		OwnerProof: nil,
	}
}
