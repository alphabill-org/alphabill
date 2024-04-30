package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/stretchr/testify/require"

	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

var (
	timeout = uint64(10)
)

type TransferFeeCreditOption func(Attributes *fc.TransferFeeCreditAttributes) TransferFeeCreditOption

func NewTransferFC(t *testing.T, attr *fc.TransferFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewTransferFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(fc.PayloadTypeTransferFeeCredit),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: maxFee,
		}),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

func NewDefaultTransferFCAttr() *fc.TransferFeeCreditAttributes {
	return &fc.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: systemID,
		TargetRecordID:         unitID,
		EarliestAdditionTime:   earliestAdditionTime,
		LatestAdditionTime:     latestAdditionTime,
		Counter:                counter,
	}
}

func NewTransferFCAttr(opts ...TransferFeeCreditOption) *fc.TransferFeeCreditAttributes {
	defaultTx := NewDefaultTransferFCAttr()
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

func WithTargetSystemID(systemID types.SystemID) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.TargetSystemIdentifier = systemID
		return nil
	}
}

func WithTargetRecordID(recordID []byte) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.TargetRecordID = recordID
		return nil
	}
}

func WithTargetUnitBacklink(targetUnitBacklink []byte) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.TargetUnitBacklink = targetUnitBacklink
		return nil
	}
}

func WithEarliestAdditionTime(earliestAdditionTime uint64) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.EarliestAdditionTime = earliestAdditionTime
		return nil
	}
}

func WithLatestAdditionTime(latestAdditionTime uint64) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.LatestAdditionTime = latestAdditionTime
		return nil
	}
}
