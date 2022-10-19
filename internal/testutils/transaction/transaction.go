package testtransaction

import (
	"math/rand"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/stretchr/testify/require"

	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"google.golang.org/protobuf/types/known/anypb"
)

var moneySystemID = []byte{0, 0, 0, 0}

func defaultTx() *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              moneySystemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                randomBytes(3),
		Timeout:               10,
		OwnerProof:            randomBytes(3),
	}
}

type Option func(*txsystem.Transaction) error

type ConvertTx func(*txsystem.Transaction) (txsystem.GenericTransaction, error)

func WithSystemID(id []byte) Option {
	return func(tx *txsystem.Transaction) error {
		tx.SystemId = id
		return nil
	}
}

func WithUnitId(id []byte) Option {
	return func(tx *txsystem.Transaction) error {
		tx.UnitId = id
		return nil
	}
}

func WithTimeout(timeout uint64) Option {
	return func(tx *txsystem.Transaction) error {
		tx.Timeout = timeout
		return nil
	}
}

func WithOwnerProof(ownerProof []byte) Option {
	return func(tx *txsystem.Transaction) error {
		tx.OwnerProof = ownerProof
		return nil
	}
}

func WithAttributes(attr proto.Message) Option {
	return func(tx *txsystem.Transaction) error {
		return tx.TransactionAttributes.MarshalFrom(attr)
	}
}

func NewTransaction(t *testing.T, options ...Option) *txsystem.Transaction {
	tx := defaultTx()
	for _, o := range options {
		require.NoError(t, o(tx))
	}
	return tx
}

func NewGenericTransaction(t *testing.T, c ConvertTx, options ...Option) txsystem.GenericTransaction {
	tx := NewTransaction(t, options...)
	genTx, err := c(tx)
	require.NoError(t, err)
	return genTx
}

func RandomGenericBillTransfer(t *testing.T) txsystem.GenericTransaction {
	return NewGenericTransaction(t, ConvertNewGenericMoneyTx, WithAttributes(&moneytx.TransferOrder{
		NewBearer: randomBytes(3),
		// #nosec G404
		TargetValue: rand.Uint64(),
		Backlink:    randomBytes(3),
	}))
}

func ConvertNewGenericMoneyTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return moneytx.NewMoneyTx(moneySystemID, tx)
}

func RandomBillTransfer(t *testing.T) *txsystem.Transaction {
	return NewTransaction(t, WithAttributes(&moneytx.TransferOrder{
		NewBearer: randomBytes(3),
		// #nosec G404
		TargetValue: rand.Uint64(),
		Backlink:    randomBytes(3),
	}))
}

func RandomBillSplit(t *testing.T) *txsystem.Transaction {
	return NewTransaction(t, WithAttributes(&moneytx.SplitOrder{
		// #nosec G404
		Amount:       rand.Uint64(),
		TargetBearer: randomBytes(3),
		// #nosec G404
		RemainingValue: rand.Uint64(),
		Backlink:       randomBytes(3),
	}))
}

func CreateBillTransferTx(pubKeyHash []byte) *anypb.Any {
	tx, _ := anypb.New(&moneytx.TransferOrder{
		TargetValue: 100,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		Backlink:    hash.Sum256([]byte{}),
	})
	return tx
}

func CreateDustTransferTx(pubKeyHash []byte) *anypb.Any {
	tx, _ := anypb.New(&moneytx.TransferDCOrder{
		TargetValue:  100,
		TargetBearer: script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		Backlink:     hash.Sum256([]byte{}),
	})
	return tx
}

func CreateBillSplitTx(pubKeyHash []byte, amount uint64, remainingValue uint64) *anypb.Any {
	tx, _ := anypb.New(&moneytx.SplitOrder{
		Amount:         amount,
		TargetBearer:   script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		RemainingValue: remainingValue,
		Backlink:       hash.Sum256([]byte{}),
	})
	return tx
}

func CreateRandomDcTx() *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              moneySystemID,
		UnitId:                hash.Sum256([]byte{0x00}),
		TransactionAttributes: CreateRandomDustTransferTx(),
		Timeout:               1000,
		OwnerProof:            script.PredicateArgumentEmpty(),
	}
}

func CreateRandomDustTransferTx() *anypb.Any {
	tx, _ := anypb.New(&moneytx.TransferDCOrder{
		TargetBearer: script.PredicateAlwaysTrue(),
		Backlink:     hash.Sum256([]byte{}),
		Nonce:        hash.Sum256([]byte{}),
		TargetValue:  100,
	})
	return tx
}

func CreateRandomSwapTransferTx(pubKeyHash []byte) *anypb.Any {
	tx, _ := anypb.New(&moneytx.SwapOrder{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		BillIdentifiers: [][]byte{},
		DcTransfers:     []*txsystem.Transaction{},
		Proofs:          []*block.BlockProof{},
		TargetValue:     100,
	})
	return tx
}

func randomBytes(len int) []byte {
	bytes := make([]byte, len)
	// #nosec G404
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}
