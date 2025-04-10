package tokenenc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
)

func RegisterAuthProof(partition types.PartitionID, reg func(id encoder.PartitionTxType, enc encoder.AuthProof) error) error {
	key := func(txType uint16) encoder.PartitionTxType {
		return encoder.PartitionTxType{
			Partition: partition,
			TxType:    txType,
		}
	}
	return errors.Join(
		reg(key(tokens.TransactionTypeTransferNFT), authProofTransferNFT),
		reg(key(tokens.TransactionTypeUpdateNFT), authProofUpdateNFT),
		reg(key(tokens.TransactionTypeBurnFT), authProofBurnFT),
		reg(key(tokens.TransactionTypeJoinFT), authProofJoinFT),
		reg(key(tokens.TransactionTypeSplitFT), authProofSplitFT),
		reg(key(tokens.TransactionTypeTransferFT), authProofTransferFT),
	)
}

func authProofTransferNFT(txo *types.TransactionOrder) ([]byte, error) {
	var authProof tokens.TransferNonFungibleTokenAuthProof
	if err := txo.UnmarshalAuthProof(&authProof); err != nil {
		return nil, fmt.Errorf("unmarshaling auth proof attributes of tx type %d: %w", txo.Type, err)
	}
	return authProof.OwnerProof, nil
}

func authProofUpdateNFT(txo *types.TransactionOrder) ([]byte, error) {
	var authProof tokens.UpdateNonFungibleTokenAuthProof
	if err := txo.UnmarshalAuthProof(&authProof); err != nil {
		return nil, fmt.Errorf("unmarshaling auth proof attributes of tx type %d: %w", txo.Type, err)
	}
	return authProof.TokenDataUpdateProof, nil
}

func authProofBurnFT(txo *types.TransactionOrder) ([]byte, error) {
	var authProof tokens.BurnFungibleTokenAuthProof
	if err := txo.UnmarshalAuthProof(&authProof); err != nil {
		return nil, fmt.Errorf("unmarshaling auth proof attributes of tx type %d: %w", txo.Type, err)
	}
	return authProof.OwnerProof, nil
}

func authProofJoinFT(txo *types.TransactionOrder) ([]byte, error) {
	var authProof tokens.JoinFungibleTokenAuthProof
	if err := txo.UnmarshalAuthProof(&authProof); err != nil {
		return nil, fmt.Errorf("unmarshaling auth proof attributes of tx type %d: %w", txo.Type, err)
	}
	return authProof.OwnerProof, nil
}

func authProofSplitFT(txo *types.TransactionOrder) ([]byte, error) {
	var authProof tokens.SplitFungibleTokenAuthProof
	if err := txo.UnmarshalAuthProof(&authProof); err != nil {
		return nil, fmt.Errorf("unmarshaling auth proof attributes of tx type %d: %w", txo.Type, err)
	}
	return authProof.OwnerProof, nil
}

func authProofTransferFT(txo *types.TransactionOrder) ([]byte, error) {
	var authProof tokens.TransferFungibleTokenAuthProof
	if err := txo.UnmarshalAuthProof(&authProof); err != nil {
		return nil, fmt.Errorf("unmarshaling auth proof attributes of tx type %d: %w", txo.Type, err)
	}
	return authProof.OwnerProof, nil
}
