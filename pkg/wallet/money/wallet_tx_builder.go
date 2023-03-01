package money

import (
	"bytes"
	"sort"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var alphabillMoneySystemId = []byte{0, 0, 0, 0}

func CreateTransactions(pubKey []byte, amount uint64, bills []*Bill, k *account.AccountKey, timeout uint64) ([]*txsystem.Transaction, error) {
	var txs []*txsystem.Transaction
	var accumulatedSum uint64
	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})
	for _, b := range bills {
		remainingAmount := amount - accumulatedSum
		tx, err := createTransaction(pubKey, k, remainingAmount, b, timeout)
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
		accumulatedSum += b.Value
		if accumulatedSum >= amount {
			return txs, nil
		}
	}
	return nil, ErrInsufficientBalance

}

func createTransaction(pubKey []byte, k *account.AccountKey, amount uint64, b *Bill, timeout uint64) (*txsystem.Transaction, error) {
	if b.Value <= amount {
		return createTransferTx(pubKey, k, b, timeout)
	}
	return createSplitTx(amount, pubKey, k, b, timeout)
}

func createTransferTx(pubKey []byte, k *account.AccountKey, bill *Bill, timeout uint64) (*txsystem.Transaction, error) {
	tx := createGenericTx(bill.GetID(), timeout)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &money.TransferOrder{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: bill.Value,
		Backlink:    bill.TxHash,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createGenericTx(unitId []byte, timeout uint64) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              alphabillMoneySystemId,
		UnitId:                unitId,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		// OwnerProof is added after whole transaction is built
	}
}

func createSplitTx(amount uint64, pubKey []byte, k *account.AccountKey, bill *Bill, timeout uint64) (*txsystem.Transaction, error) {
	tx := createGenericTx(bill.GetID(), timeout)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &money.SplitOrder{
		Amount:         amount,
		TargetBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		RemainingValue: bill.Value - amount,
		Backlink:       bill.TxHash,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func CreateDustTx(k *account.AccountKey, bill *Bill, nonce []byte, timeout uint64) (*txsystem.Transaction, error) {
	tx := createGenericTx(bill.GetID(), timeout)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &money.TransferDCOrder{
		TargetValue:  bill.Value,
		TargetBearer: script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256),
		Backlink:     bill.TxHash,
		Nonce:        nonce,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func CreateSwapTx(k *account.AccountKey, dcBills []*Bill, dcNonce []byte, billIds [][]byte, timeout uint64) (*txsystem.Transaction, error) {
	if len(dcBills) == 0 {
		return nil, errors.New("cannot create swap transaction as no dust bills exist")
	}
	// sort bills by ids in ascending order
	sort.Slice(billIds, func(i, j int) bool {
		return bytes.Compare(billIds[i], billIds[j]) < 0
	})
	sort.Slice(dcBills, func(i, j int) bool {
		return bytes.Compare(dcBills[i].GetID(), dcBills[j].GetID()) < 0
	})

	var dustTransferProofs []*block.BlockProof
	var dustTransferOrders []*txsystem.Transaction
	var billValueSum uint64
	for _, b := range dcBills {
		dustTransferOrders = append(dustTransferOrders, b.BlockProof.Tx)
		dustTransferProofs = append(dustTransferProofs, b.BlockProof.Proof)
		billValueSum += b.Value
	}

	swapTx := createGenericTx(dcNonce, timeout)
	err := anypb.MarshalFrom(swapTx.TransactionAttributes, &money.SwapOrder{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256),
		BillIdentifiers: billIds,
		DcTransfers:     dustTransferOrders,
		Proofs:          dustTransferProofs,
		TargetValue:     billValueSum,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(swapTx, k)
	if err != nil {
		return nil, err
	}
	return swapTx, nil
}

func signTx(tx *txsystem.Transaction, ac *account.AccountKey) error {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return err
	}
	gtx, err := money.NewMoneyTx(alphabillMoneySystemId, tx)
	if err != nil {
		return err
	}
	sig, err := signer.SignBytes(gtx.SigBytes())
	if err != nil {
		return err
	}
	tx.OwnerProof = script.PredicateArgumentPayToPublicKeyHashDefault(sig, ac.PubKey)
	return nil
}
