package contracts

import (
	"bytes"
	"crypto"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"

	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/types"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/ethereum/go-ethereum/common"
)

const AlphabillLibABI = "[\n\t{\n\t\t\"inputs\": [\n\t\t\t{\n\t\t\t\t\"internalType\": \"bytes\",\n\t\t\t\t\"name\": \"txRecord\",\n\t\t\t\t\"type\": \"bytes\"\n\t\t\t},\n\t\t\t{\n\t\t\t\t\"internalType\": \"bytes\",\n\t\t\t\t\"name\": \"txProof\",\n\t\t\t\t\"type\": \"bytes\"\n\t\t\t}\n\t\t],\n\t\t\"name\": \"verifyTxRecordProof\",\n\t\t\"outputs\": [\n\t\t\t{\n\t\t\t\t\"internalType\": \"bool\",\n\t\t\t\t\"name\": \"\",\n\t\t\t\t\"type\": \"bool\"\n\t\t\t}\n\t\t],\n\t\t\"stateMutability\": \"view\",\n\t\t\"type\": \"function\"\n\t}\n]"

type (
	AlphabillLibPrecompiledContract struct {
		abi      abi.ABI
		gas      map[string]uint64
		executor map[string]ContractRunner
	}

	ContractRunner func(input []byte, caller common.Address) ([]byte, error)
)

func NewAlphabillLibContract(trustBase map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) *AlphabillLibPrecompiledContract {
	a, err := abi.JSON(bytes.NewBuffer([]byte(AlphabillLibABI)))
	if err != nil {
		panic(fmt.Sprintf("Unable to load AlphabillLib contract ABI: %v", err))
	}
	return &AlphabillLibPrecompiledContract{
		abi: a,
		gas: map[string]uint64{
			"verifyTxRecordProof": 1000,
		},
		executor: map[string]ContractRunner{
			"verifyTxRecordProof": verifyTxRecordProof(a, trustBase, hashAlgorithm),
		},
	}
}

func (d *AlphabillLibPrecompiledContract) RequiredGas(input []byte) uint64 {
	method, err := d.abi.MethodById(input)
	if err != nil {
		// function not present in ABI
		return 0
	}
	return d.gas[method.Name]
}

func (d *AlphabillLibPrecompiledContract) Run(input []byte, caller common.Address) ([]byte, error) {
	method, err := d.abi.MethodById(input)
	if err != nil {
		return nil, fmt.Errorf("invalid method: %w", err)
	}
	return d.executor[method.Name](input, caller)
}

func verifyTxRecordProof(a abi.ABI, trustBase map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) ContractRunner {
	return func(input []byte, caller common.Address) ([]byte, error) {
		res, err := UnpackInput(a, "verifyTxRecordProof", input[4:])
		if err != nil {
			return nil, fmt.Errorf("unable to unpack 'verifyTxRecordProof' input data: %w", err)
		}

		txrBytes := *abi.ConvertType(res[0], new([]byte)).(*[]byte)
		proofBytes := *abi.ConvertType(res[1], new([]byte)).(*[]byte)

		txr := &types.TransactionRecord{}
		if err := cbor.Unmarshal(txrBytes, txr); err != nil {
			output, e := PackOutput(a, "verifyTxRecordProof", false)
			if e != nil {
				return nil, e
			}
			return output, fmt.Errorf("invalid transaction record data: %w", err)
		}

		proof := &types.TxProof{}
		if err := cbor.Unmarshal(proofBytes, proof); err != nil {
			output, e := PackOutput(a, "verifyTxRecordProof", false)
			if e != nil {
				return nil, e
			}
			return output, fmt.Errorf("invalid transaction execution proof data: %w", err)
		}

		if err = types.VerifyTxProof(proof, txr, trustBase, hashAlgorithm); err != nil {
			output, e := PackOutput(a, "verifyTxRecordProof", false)
			if e != nil {
				return nil, e
			}
			return output, fmt.Errorf("invalid transaction execution proof: %w", err)
		}

		return PackOutput(a, "verifyTxRecordProof", true)
	}
}
