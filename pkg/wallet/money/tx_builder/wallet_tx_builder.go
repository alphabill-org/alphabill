package tx_builder

import (
	"bytes"
	gocrypto "crypto"
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
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const MaxFee = uint64(1)

type txBatchAdder func(tx *txsubmitter.TxSubmission)

var ErrInsufficientBalance = errors.New("insufficient balance for transaction")

func CreateTransactions(add txBatchAdder, txConverter *TxConverter, targetPubKey []byte, amount uint64, systemId []byte, bills []*bp.Bill, k *account.AccountKey, timeout uint64, fcrID []byte) error {
	//TODO: func CreateTransactions(pubKey []byte, amount uint64, systemId []byte, bills []*bp.Bill, k *account.AccountKey, timeout uint64, fcrID []byte) ([]*txsystem.Transaction, error) {
	var accumulatedSum uint64
	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})

	for _, b := range bills {
		remainingAmount := amount - accumulatedSum
		tx, err := CreateTransaction(targetPubKey, k, remainingAmount, systemId, b, timeout, fcrID)
		if err != nil {
			return err
		}
		gtx, err := txConverter.ConvertTx(tx)
		if err != nil {
			return err
		}
		add(&txsubmitter.TxSubmission{
			UnitID:      b.GetId(),
			TxHash:      gtx.Hash(gocrypto.SHA256),
			Transaction: tx,
		})
		accumulatedSum += b.Value
		if accumulatedSum >= amount {
			return nil
		}
	}
	return ErrInsufficientBalance
}

func CreateTransaction(pubKey []byte, k *account.AccountKey, amount uint64, systemId []byte, b *bp.Bill, timeout uint64, fcrID []byte) (*txsystem.Transaction, error) {
	if b.Value <= amount {
		return CreateTransferTx(pubKey, k, systemId, b, timeout, fcrID)
	}
	return CreateSplitTx(amount, pubKey, k, systemId, b, timeout, fcrID)
}

func CreateTransferTx(pubKey []byte, k *account.AccountKey, systemId []byte, bill *bp.Bill, timeout uint64, fcrID []byte) (*txsystem.Transaction, error) {
	tx := CreateGenericTx(systemId, bill.GetId(), timeout, fcrID)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &money.TransferAttributes{
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

func CreateTransferFCTx(amount uint64, targetRecordID []byte, nonce []byte, k *account.AccountKey, moneySystemID, targetSystemID []byte, unit *bp.Bill, timeout, t1, t2 uint64) (*txsystem.Transaction, error) {
	tx := CreateGenericTx(moneySystemID, unit.GetId(), timeout, nil)
	transferFC := &transactions.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: targetSystemID,
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
	err = signTx(moneySystemID, tx, k)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func CreateAddFCTx(unitID []byte, fcProof *block.TxProof, k *account.AccountKey, systemId []byte, timeout uint64) (*txsystem.Transaction, error) {
	tx := CreateGenericTx(systemId, unitID, timeout, nil)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &transactions.AddFeeCreditAttributes{
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

func CreateCloseFCTx(systemId []byte, unitID []byte, timeout uint64, amount uint64, targetUnitID, nonce []byte, k *account.AccountKey) (*txsystem.Transaction, error) {
	tx := CreateGenericTx(systemId, unitID, timeout, nil)
	closeFC := &transactions.CloseFeeCreditAttributes{
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

func CreateReclaimFCTx(systemID []byte, unitID []byte, timeout uint64, fcProof *block.TxProof, backlink []byte, k *account.AccountKey) (*txsystem.Transaction, error) {
	tx := CreateGenericTx(systemID, unitID, timeout, nil)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &transactions.ReclaimFeeCreditAttributes{
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

func CreateGenericTx(systemId, unitId []byte, timeout uint64, fcrID []byte) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              systemId,
		UnitId:                unitId,
		TransactionAttributes: new(anypb.Any),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout:           timeout,
			MaxFee:            MaxFee,
			FeeCreditRecordId: fcrID,
		},
		// OwnerProof is added after whole transaction is built
	}
}

func CreateSplitTx(amount uint64, pubKey []byte, k *account.AccountKey, systemId []byte, bill *bp.Bill, timeout uint64, fcrID []byte) (*txsystem.Transaction, error) {
	tx := CreateGenericTx(systemId, bill.GetId(), timeout, fcrID)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &money.SplitAttributes{
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

func CreateDustTx(k *account.AccountKey, systemId []byte, bill *bp.Bill, nonce []byte, timeout uint64) (*txsystem.Transaction, error) {
	tx := CreateGenericTx(systemId, bill.GetId(), timeout, k.PrivKeyHash)
	err := anypb.MarshalFrom(tx.TransactionAttributes, &money.TransferDCAttributes{
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

func CreateSwapTx(k *account.AccountKey, systemId []byte, dcBills []*bp.Bill, dcNonce []byte, billIds [][]byte, timeout uint64) (*txsystem.Transaction, error) {
	if len(dcBills) == 0 {
		return nil, errors.New("cannot create swap transaction as no dust bills exist")
	}
	// sort bills by ids in ascending order
	sort.Slice(billIds, func(i, j int) bool {
		return bytes.Compare(billIds[i], billIds[j]) < 0
	})
	sort.Slice(dcBills, func(i, j int) bool {
		return bytes.Compare(dcBills[i].GetId(), dcBills[j].GetId()) < 0
	})

	var dustTransferProofs []*block.BlockProof
	var dustTransferOrders []*txsystem.Transaction
	var billValueSum uint64
	for _, b := range dcBills {
		dustTransferOrders = append(dustTransferOrders, b.TxProof.Tx)
		dustTransferProofs = append(dustTransferProofs, b.TxProof.Proof)
		billValueSum += b.Value
	}

	swapTx := CreateGenericTx(systemId, dcNonce, timeout, k.PrivKeyHash)
	err := anypb.MarshalFrom(swapTx.TransactionAttributes, &money.SwapDCAttributes{
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
