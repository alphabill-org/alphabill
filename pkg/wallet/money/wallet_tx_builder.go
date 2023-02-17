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
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const maxFee = uint64(1)

func createTransactions(pubKey []byte, amount uint64, systemId []byte, bills []*Bill, k *account.AccountKey, timeout uint64, fcrID []byte) ([]*txsystem.Transaction, error) {
	var txs []*txsystem.Transaction
	var accumulatedSum uint64
	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})
	for _, b := range bills {
		remainingAmount := amount - accumulatedSum
		tx, err := createTransaction(pubKey, k, remainingAmount, systemId, b, timeout, fcrID)
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

func createTransaction(pubKey []byte, k *account.AccountKey, amount uint64, systemId []byte, b *Bill, timeout uint64, fcrID []byte) (*txsystem.Transaction, error) {
	if b.Value <= amount {
		return createTransferTx(pubKey, k, systemId, b, timeout, fcrID)
	}
	return createSplitTx(amount, pubKey, k, systemId, b, timeout, fcrID)
}

func createTransferTx(pubKey []byte, k *account.AccountKey, systemId []byte, bill *Bill, timeout uint64, fcrID []byte) (*txsystem.Transaction, error) {
	tx := createGenericTx(systemId, bill.GetID(), timeout, fcrID)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &money.TransferOrder{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: bill.Value,
		Backlink:    bill.TxHash,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(systemId, tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createTransferFCTx(amount uint64, targetRecordID []byte, nonce []byte, k *account.AccountKey, systemId []byte, unit *Bill, t1, t2 uint64) (*txsystem.Transaction, error) {
	tx := createGenericTx(systemId, unit.GetID(), t2, nil)
	transferFC := &transactions.TransferFeeCreditOrder{
		Amount:                 amount,
		TargetSystemIdentifier: systemId,
		TargetRecordId:         targetRecordID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
		Nonce:                  nonce,
		Backlink:               unit.TxHash,
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, transferFC, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(systemId, tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createAddFCTx(unitID []byte, fcProof *BlockProof, k *account.AccountKey, systemId []byte, timeout uint64) (*txsystem.Transaction, error) {
	tx := createGenericTx(systemId, unitID, timeout, nil)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &transactions.AddFeeCreditOrder{
		FeeCreditOwnerCondition: script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256),
		FeeCreditTransfer:       fcProof.Tx,
		FeeCreditTransferProof:  fcProof.Proof,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(systemId, tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createCloseFCTx(systemId []byte, unitID []byte, timeout uint64, amount uint64, targetUnitID, nonce []byte, k *account.AccountKey) (*txsystem.Transaction, error) {
	tx := createGenericTx(systemId, unitID, timeout, nil)
	closeFC := &transactions.CloseFeeCreditOrder{
		Amount:       amount,
		TargetUnitId: targetUnitID,
		Nonce:        nonce,
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, closeFC, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(systemId, tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createReclaimFCTx(systemID []byte, unitID []byte, timeout uint64, fcProof *BlockProof, backlink []byte, k *account.AccountKey) (*txsystem.Transaction, error) {
	tx := createGenericTx(systemID, unitID, timeout, nil)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &transactions.ReclaimFeeCreditOrder{
		CloseFeeCreditTransfer: fcProof.Tx,
		CloseFeeCreditProof:    fcProof.Proof,
		Backlink:               backlink,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(systemID, tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createGenericTx(systemId, unitId []byte, timeout uint64, fcrID []byte) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              systemId,
		UnitId:                unitId,
		TransactionAttributes: new(anypb.Any),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout:           timeout,
			MaxFee:            maxFee,
			FeeCreditRecordId: fcrID,
		},
		// OwnerProof is added after whole transaction is built
	}
}

func createSplitTx(amount uint64, pubKey []byte, k *account.AccountKey, systemId []byte, bill *Bill, timeout uint64, fcrID []byte) (*txsystem.Transaction, error) {
	tx := createGenericTx(systemId, bill.GetID(), timeout, fcrID)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &money.SplitOrder{
		Amount:         amount,
		TargetBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		RemainingValue: bill.Value - amount,
		Backlink:       bill.TxHash,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(systemId, tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createDustTx(k *account.AccountKey, systemId []byte, bill *Bill, nonce []byte, timeout uint64) (*txsystem.Transaction, error) {
	tx := createGenericTx(systemId, bill.GetID(), timeout, k.PrivKeyHash)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &money.TransferDCOrder{
		TargetValue:  bill.Value,
		TargetBearer: script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256),
		Backlink:     bill.TxHash,
		Nonce:        nonce,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	err = signTx(systemId, tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createSwapTx(k *account.AccountKey, systemId []byte, dcBills []*Bill, dcNonce []byte, billIds [][]byte, timeout uint64) (*txsystem.Transaction, error) {
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

	swapTx := createGenericTx(systemId, dcNonce, timeout, k.PrivKeyHash)
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
	err = signTx(systemId, swapTx, k)
	if err != nil {
		return nil, err
	}
	return swapTx, nil
}

func signTx(systemId []byte, tx *txsystem.Transaction, ac *account.AccountKey) error {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return err
	}
	gtx, err := money.NewMoneyTx(systemId, tx)
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
