package tx_builder

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/fxamacker/cbor/v2"
)

const MaxFee = uint64(1)

func CreateTransactions(pubKey []byte, amount uint64, systemID []byte, bills []*wallet.Bill, k *account.AccountKey, timeout uint64, fcrID []byte) ([]*types.TransactionOrder, error) {
	var txs []*types.TransactionOrder
	var accumulatedSum uint64
	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})
	for _, b := range bills {
		remainingAmount := amount - accumulatedSum
		tx, err := CreateTransaction(pubKey, k, remainingAmount, systemID, b, timeout, fcrID)
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
		accumulatedSum += b.Value
		if accumulatedSum >= amount {
			return txs, nil
		}
	}
	return nil, fmt.Errorf("insufficient balance for transaction, trying to send %d have %d", amount, accumulatedSum)
}

func CreateTransaction(receiverPubKey []byte, k *account.AccountKey, amount uint64, systemID []byte, b *wallet.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	if b.Value <= amount {
		return NewTransferTx(receiverPubKey, k, systemID, b, timeout, fcrID)
	}
	return NewSplitTx(amount, receiverPubKey, k, systemID, b, timeout, fcrID)
}

func NewTransferTx(receiverPubKey []byte, k *account.AccountKey, systemID []byte, bill *wallet.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(receiverPubKey)),
		TargetValue: bill.Value,
		Backlink:    bill.TxRecordHash,
	}
	txPayload, err := newTxPayload(systemID, money.PayloadTypeTransfer, bill.GetID(), timeout, fcrID, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, k)
}

func NewSplitTx(amount uint64, pubKey []byte, k *account.AccountKey, systemID []byte, bill *wallet.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	attr := &money.SplitAttributes{
		Amount:         amount,
		TargetBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		RemainingValue: bill.Value - amount,
		Backlink:       bill.TxRecordHash,
	}
	txPayload, err := newTxPayload(systemID, money.PayloadTypeSplit, bill.GetID(), timeout, fcrID, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, k)
}

func NewDustTx(ac *account.AccountKey, systemID []byte, bill *wallet.Bill, nonce []byte, timeout uint64) (*types.TransactionOrder, error) {
	attr := &money.TransferDCAttributes{
		TargetValue:  bill.Value,
		TargetBearer: script.PredicatePayToPublicKeyHashDefault(ac.PubKeyHash.Sha256),
		Backlink:     bill.TxRecordHash,
		Nonce:        nonce,
	}
	txPayload, err := newTxPayload(systemID, money.PayloadTypeTransDC, bill.GetID(), timeout, ac.PrivKeyHash, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, ac)
}

func NewSwapTx(k *account.AccountKey, systemID []byte, dcBills []*wallet.Bill, dcNonce []byte, billIds [][]byte, timeout uint64) (*types.TransactionOrder, error) {
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

	var dustTransferProofs []*types.TxProof
	var dustTransferRecords []*types.TransactionRecord
	var billValueSum uint64
	for _, b := range dcBills {
		dustTransferRecords = append(dustTransferRecords, b.TxProof.TxRecord)
		dustTransferProofs = append(dustTransferProofs, b.TxProof.TxProof)
		billValueSum += b.Value
	}
	attr := &money.SwapDCAttributes{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256),
		BillIdentifiers: billIds,
		DcTransfers:     dustTransferRecords,
		Proofs:          dustTransferProofs,
		TargetValue:     billValueSum,
	}
	swapTx, err := newTxPayload(systemID, money.PayloadTypeSwapDC, dcNonce, timeout, k.PrivKeyHash, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(swapTx, k)
}

func NewTransferFCTx(amount uint64, targetRecordID []byte, nonce []byte, k *account.AccountKey, moneySystemID, targetSystemID []byte, unit *wallet.Bill, timeout, t1, t2 uint64) (*types.TransactionOrder, error) {
	attr := &transactions.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: targetSystemID,
		TargetRecordID:         targetRecordID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
		Nonce:                  nonce,
		Backlink:               unit.TxRecordHash,
	}
	txPayload, err := newTxPayload(moneySystemID, transactions.PayloadTypeTransferFeeCredit, unit.GetID(), timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, k)
}

func NewAddFCTx(unitID []byte, fcProof *wallet.Proof, ac *account.AccountKey, systemID []byte, timeout uint64) (*types.TransactionOrder, error) {
	attr := &transactions.AddFeeCreditAttributes{
		FeeCreditOwnerCondition: script.PredicatePayToPublicKeyHashDefault(ac.PubKeyHash.Sha256),
		FeeCreditTransfer:       fcProof.TxRecord,
		FeeCreditTransferProof:  fcProof.TxProof,
	}
	txPayload, err := newTxPayload(systemID, transactions.PayloadTypeAddFeeCredit, unitID, timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, ac)
}

func NewCloseFCTx(systemID []byte, unitID []byte, timeout uint64, amount uint64, targetUnitID, nonce []byte, k *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &transactions.CloseFeeCreditAttributes{
		Amount:       amount,
		TargetUnitID: targetUnitID,
		Nonce:        nonce,
	}
	txPayload, err := newTxPayload(systemID, transactions.PayloadTypeCloseFeeCredit, unitID, timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, k)
}

func NewReclaimFCTx(systemID []byte, unitID []byte, timeout uint64, fcProof *wallet.Proof, backlink []byte, k *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &transactions.ReclaimFeeCreditAttributes{
		CloseFeeCreditTransfer: fcProof.TxRecord,
		CloseFeeCreditProof:    fcProof.TxProof,
		Backlink:               backlink,
	}
	txPayload, err := newTxPayload(systemID, transactions.PayloadTypeReclaimFeeCredit, unitID, timeout, nil, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, k)
}

func newTxPayload(systemID []byte, txType string, unitID []byte, timeout uint64, fcrID []byte, attr interface{}) (*types.Payload, error) {
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	return &types.Payload{
		SystemID:   systemID,
		Type:       txType,
		UnitID:     unitID,
		Attributes: attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: MaxFee,
			FeeCreditRecordID: fcrID,
		},
	}, nil
}

func signPayload(payload *types.Payload, ac *account.AccountKey) (*types.TransactionOrder, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := payload.Bytes()
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(payloadBytes)
	if err != nil {
		return nil, err
	}
	return &types.TransactionOrder{
		Payload:    payload,
		OwnerProof: script.PredicateArgumentPayToPublicKeyHashDefault(sig, ac.PubKey),
	}, nil
}
