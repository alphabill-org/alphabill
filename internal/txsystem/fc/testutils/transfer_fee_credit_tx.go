package testutils

import (
	"testing"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/stretchr/testify/require"
)

var (
	timeout = uint64(10)
)

type TransferFCOption func(Attributes *transactions.TransferFeeCreditAttributes) TransferFCOption

func NewTransferFC(t *testing.T, attr *transactions.TransferFeeCreditAttributes, opts ...testtransaction.Option) *transactions.TransferFeeCreditWrapper {
	if attr == nil {
		attr = NewTransferFCAttr()
	}
	defaultTx := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout: timeout,
			MaxFee:  maxFee,
		}),
	)
	for _, opt := range opts {
		require.NoError(t, opt(defaultTx))
	}
	tx, err := transactions.NewFeeCreditTx(defaultTx)
	require.NoError(t, err)

	return tx.(*transactions.TransferFeeCreditWrapper)
}

func NewDefaultTransferFCAttr() *transactions.TransferFeeCreditAttributes {
	return &transactions.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: systemID,
		TargetRecordId:         unitID,
		EarliestAdditionTime:   earliestAdditionTime,
		LatestAdditionTime:     latestAdditionTime,
		Backlink:               backlink,
	}
}

func NewTransferFCAttr(opts ...TransferFCOption) *transactions.TransferFeeCreditAttributes {
	defaultTx := NewDefaultTransferFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func WithAmount(amount uint64) TransferFCOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFCOption {
		tx.Amount = amount
		return nil
	}
}

func WithBacklink(backlink []byte) TransferFCOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFCOption {
		tx.Backlink = backlink
		return nil
	}
}

func WithTargetSystemID(systemID []byte) TransferFCOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFCOption {
		tx.TargetSystemIdentifier = systemID
		return nil
	}
}

func WithTargetRecordID(recordID []byte) TransferFCOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFCOption {
		tx.TargetRecordId = recordID
		return nil
	}
}

func WithNonce(nonce []byte) TransferFCOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFCOption {
		tx.Nonce = nonce
		return nil
	}
}

func WithEarliestAdditionTime(earliestAdditionTime uint64) TransferFCOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFCOption {
		tx.EarliestAdditionTime = earliestAdditionTime
		return nil
	}
}

func WithLatestAdditionTime(latestAdditionTime uint64) TransferFCOption {
	return func(tx *transactions.TransferFeeCreditAttributes) TransferFCOption {
		tx.LatestAdditionTime = latestAdditionTime
		return nil
	}
}
