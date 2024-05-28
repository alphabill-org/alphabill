package testutils

import (
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

var (
	timeout = uint64(10)
)

type TransferFeeCreditOption func(Attributes *fc.TransferFeeCreditAttributes)

func NewTransferFC(t *testing.T, signer abcrypto.Signer, attr *fc.TransferFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewTransferFCAttr(t, signer)
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(DefaultMoneyUnitID()),
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

func NewDefaultTransferFCAttr(t *testing.T, signer abcrypto.Signer) *fc.TransferFeeCreditAttributes {
	return &fc.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: systemID,
		TargetRecordID:         NewFeeCreditRecordID(t, signer),
		LatestAdditionTime:     latestAdditionTime,
		Counter:                counter,
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
	return func(tx *fc.TransferFeeCreditAttributes) {
		tx.Amount = amount
	}
}

func WithCounter(counter uint64) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) {
		tx.Counter = counter
	}
}

func WithTargetSystemID(systemID types.SystemID) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) {
		tx.TargetSystemIdentifier = systemID
	}
}

func WithTargetRecordID(recordID []byte) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) {
		tx.TargetRecordID = recordID
	}
}

func WithTargetUnitCounter(targetUnitCounter uint64) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) {
		tx.TargetUnitCounter = &targetUnitCounter
	}
}

func WithLatestAdditionTime(latestAdditionTime uint64) TransferFeeCreditOption {
	return func(tx *fc.TransferFeeCreditAttributes) {
		tx.LatestAdditionTime = latestAdditionTime
	}
}
