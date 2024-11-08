package testutils

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"

	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

var (
	timeout = uint64(10)
)

type TransferFeeCreditOption func(Attributes *fc.TransferFeeCreditAttributes) TransferFeeCreditOption

func NewTransferFC(t *testing.T, signer abcrypto.Signer, attr *fc.TransferFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewTransferFCAttr(t, signer)
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(DefaultMoneyUnitID()),
		testtransaction.WithAttributes(attr),
		testtransaction.WithTransactionType(fc.TransactionTypeTransferFeeCredit),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: maxFee,
		}),
		testtransaction.WithAuthProof(fc.TransferFeeCreditAuthProof{}),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

func NewDefaultTransferFCAttr(t *testing.T, signer abcrypto.Signer) *fc.TransferFeeCreditAttributes {
	return &fc.TransferFeeCreditAttributes{
		Amount:             amount,
		TargetPartitionID:  partitionID,
		TargetRecordID:     NewFeeCreditRecordID(t, signer),
		LatestAdditionTime: latestAdditionTime,
		Counter:            counter,
	}
}

func NewTransferFCAttr(t *testing.T, signer abcrypto.Signer, opts ...TransferFeeCreditOption) *fc.TransferFeeCreditAttributes {
	defaultTx := NewDefaultTransferFCAttr(t, signer)
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func WithAmount(amount uint64) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.Amount = amount
		return nil
	}
}

func WithCounter(counter uint64) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.Counter = counter
		return nil
	}
}

func WithTargetPartitionID(partitionID types.PartitionID) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.TargetPartitionID = partitionID
		return nil
	}
}

func WithTargetRecordID(recordID []byte) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.TargetRecordID = recordID
		return nil
	}
}

func WithTargetUnitCounter(targetUnitCounter uint64) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.TargetUnitCounter = &targetUnitCounter
		return nil
	}
}

func WithLatestAdditionTime(latestAdditionTime uint64) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.LatestAdditionTime = latestAdditionTime
		return nil
	}
}
