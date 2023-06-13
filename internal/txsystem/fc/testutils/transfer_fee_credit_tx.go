package testutils

import (
	"testing"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

var (
	timeout = uint64(10)
)

type TransferFeeCreditOption func(Attributes *transactions.TransferFeeCreditAttributes) TransferFeeCreditOption

func NewTransferFC(t *testing.T, attr *transactions.TransferFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewTransferFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(transactions.PayloadTypeTransferFeeCredit),
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

func NewDefaultTransferFCAttr() *transactions.TransferFeeCreditAttributes {
	return &transactions.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: systemID,
		TargetRecordID:         unitID,
		EarliestAdditionTime:   earliestAdditionTime,
		LatestAdditionTime:     latestAdditionTime,
		Backlink:               backlink,
	}
}

func NewTransferFCAttr(opts ...TransferFeeCreditOption) *transactions.TransferFeeCreditAttributes {
	defaultTx := NewDefaultTransferFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func WithAmount(amount uint64) TransferFeeCreditOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.Amount = amount
		return nil
	}
}

func WithBacklink(backlink []byte) TransferFeeCreditOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.Backlink = backlink
		return nil
	}
}

func WithTargetSystemID(systemID []byte) TransferFeeCreditOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.TargetSystemIdentifier = systemID
		return nil
	}
}

func WithTargetRecordID(recordID []byte) TransferFeeCreditOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.TargetRecordID = recordID
		return nil
	}
}

func WithNonce(nonce []byte) TransferFeeCreditOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.Nonce = nonce
		return nil
	}
}

func WithEarliestAdditionTime(earliestAdditionTime uint64) TransferFeeCreditOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.EarliestAdditionTime = earliestAdditionTime
		return nil
	}
}

func WithLatestAdditionTime(latestAdditionTime uint64) TransferFeeCreditOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFeeCreditOption {
		tx.LatestAdditionTime = latestAdditionTime
		return nil
	}
}
