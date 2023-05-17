package money

import (
	"math/rand"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
)

/*port (
	"math/rand"
	"testing"

	"github.com/alphabill-org/alphabill/internal/types"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"google.golang.org/protobuf/types/known/anypb"
)

func RandomGenericBillTransfer(t *testing.T) *types.TransactionRecord {
	return NewTransferTx(t,
		// #nosec G404
		rand.Uint64(),
		testtransaction.RandomBytes(3),
		testtransaction.RandomBytes(3),
	)
}

func NewTransferTx(t *testing.T, targetValue uint64, bearer, backlink []byte) *types.TransactionRecord {
	return testtransaction.NewGenericTransaction(t, ConvertNewGenericMoneyTx, testtransaction.WithAttributes(&moneytx.TransferAttributes{
		NewBearer:   bearer,
		TargetValue: targetValue,
		Backlink:    backlink,
	}))
}

func ConvertNewGenericMoneyTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return moneytx.NewMoneyTx([]byte{0, 0, 0, 0}, tx)
}

func RandomBillTransfer(t *testing.T) *types.TransactionRecord {
	return testtransaction.NewTransaction(t, testtransaction.WithAttributes(RandomTransferAttributes()))
}

func RandomBillSplit(t *testing.T) *types.TransactionRecord {
	return testtransaction.NewTransaction(t, testtransaction.WithAttributes(RandomSplitAttributes()))
}

func CreateBillTransferTx(pubKeyHash []byte) *anypb.Any {
	tx, _ := anypb.New(&moneytx.TransferAttributes{
		TargetValue: 100,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		Backlink:    hash.Sum256([]byte{}),
	})
	return tx
}

func CreateDustTransferTx(pubKeyHash []byte) *anypb.Any {
	tx, _ := anypb.New(&moneytx.TransferDCAttributes{
		TargetValue:  100,
		TargetBearer: script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		Backlink:     hash.Sum256([]byte{}),
	})
	return tx
}

func CreateBillSplitTx(pubKeyHash []byte, amount uint64, remainingValue uint64) *anypb.Any {
	tx, _ := anypb.New(&moneytx.SplitAttributes{
		Amount:         amount,
		TargetBearer:   script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		RemainingValue: remainingValue,
		Backlink:       hash.Sum256([]byte{}),
	})
	return tx
}

func CreateRandomDcTx() *types.TransactionRecord {
	return &txsystem.Transaction{
		SystemId:              []byte{0, 0, 0, 0},
		UnitId:                hash.Sum256([]byte{0x00}),
		TransactionAttributes: CreateRandomDustTransferTx(),
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: 1000},
		OwnerProof:            script.PredicateArgumentEmpty(),
	}
}

func CreateRandomDustTransferTx() *anypb.Any {
	tx, _ := anypb.New(RandomTransferDCAttributes())
	return tx
}

func CreateRandomSwapTransferTx(pubKeyHash []byte) *anypb.Any {
	tx, _ := anypb.New(&moneytx.SwapDCAttributes{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		BillIdentifiers: [][]byte{},
		DcTransfers:     []*txsystem.Transaction{},
		Proofs:          []*block.BlockProof{},
		TargetValue:     100,
	})
	return tx
}

func RandomTransferDCAttributes() *moneytx.TransferDCAttributes {
	return &moneytx.TransferDCAttributes{
		TargetBearer: script.PredicateAlwaysTrue(),
		Backlink:     hash.Sum256([]byte{}),
		Nonce:        hash.Sum256([]byte{}),
		TargetValue:  100,
	}
}

func RandomSplitAttributes() *moneytx.SplitAttributes {
	return &moneytx.SplitAttributes{
		// #nosec G404
		Amount:       rand.Uint64(),
		TargetBearer: testtransaction.RandomBytes(3),
		// #nosec G404
		RemainingValue: rand.Uint64(),
		Backlink:       testtransaction.RandomBytes(3),
	}
}*/

func RandomTransferAttributes() *moneytx.TransferAttributes {
	return &moneytx.TransferAttributes{
		NewBearer: testtransaction.RandomBytes(3),
		// #nosec G404
		TargetValue: rand.Uint64(),
		Backlink:    testtransaction.RandomBytes(3),
	}
}

/*
func CreateRandomSwapDCAttributes(t *testing.T, txCount int) *moneytx.SwapDCAttributes {
	signer, _ := testsig.CreateSignerAndVerifier(t)
	swap := &moneytx.SwapDCAttributes{
		OwnerCondition: script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32)),
		TargetValue:    100,
	}
	var gtxs []*types.TransactionRecord
	for i := 0; i < txCount; i++ {
		tx := CreateRandomDcTx()
		swap.DcTransfers = append(swap.DcTransfers, tx)
		swap.BillIdentifiers = append(swap.BillIdentifiers, tx.TransactionOrder.UnitID())
		gtxs = append(gtxs, tx)
	}
	swap.Proofs = testblock.CreatePrimaryProofs(t, gtxs, signer)
	return swap
}
*/
