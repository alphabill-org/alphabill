package money

import (
	"math/rand"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"google.golang.org/protobuf/types/known/anypb"
)

func RandomGenericBillTransfer(t *testing.T) txsystem.GenericTransaction {
	return testtransaction.NewGenericTransaction(t, ConvertNewGenericMoneyTx, testtransaction.WithAttributes(&moneytx.TransferOrder{
		NewBearer: testtransaction.RandomBytes(3),
		// #nosec G404
		TargetValue: rand.Uint64(),
		Backlink:    testtransaction.RandomBytes(3),
	}))
}

func ConvertNewGenericMoneyTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return moneytx.NewMoneyTx([]byte{0, 0, 0, 0}, tx)
}

func RandomBillTransfer(t *testing.T) *txsystem.Transaction {
	return testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		NewBearer: testtransaction.RandomBytes(3),
		// #nosec G404
		TargetValue: rand.Uint64(),
		Backlink:    testtransaction.RandomBytes(3),
	}))
}

func RandomBillSplit(t *testing.T) *txsystem.Transaction {
	return testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.SplitOrder{
		// #nosec G404
		Amount:       rand.Uint64(),
		TargetBearer: testtransaction.RandomBytes(3),
		// #nosec G404
		RemainingValue: rand.Uint64(),
		Backlink:       testtransaction.RandomBytes(3),
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
		SystemId:              []byte{0, 0, 0, 0},
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
