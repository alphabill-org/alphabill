package tx_builder

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/exp/slices"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
)

const MaxFee = uint64(1)

// CreateTransactions creates 1 to N P2PKH transactions from given bills until target amount is reached.
// If there exists a bill with value equal to the given amount then transfer transaction is created using that bill,
// otherwise bills are selected in the given order.
func CreateTransactions(pubKey []byte, amount uint64, systemID []byte, bills []*wallet.Bill, k *account.AccountKey, timeout uint64, fcrID []byte) ([]*types.TransactionOrder, error) {
	billIndex := slices.IndexFunc(bills, func(b *wallet.Bill) bool { return b.Value == amount })
	if billIndex >= 0 {
		tx, err := NewTransferTx(pubKey, k, systemID, bills[billIndex], timeout, fcrID)
		if err != nil {
			return nil, err
		}
		return []*types.TransactionOrder{tx}, nil
	}
	var txs []*types.TransactionOrder
	var accumulatedSum uint64
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

// CreateTransaction creates a P2PKH transfer or split transaction using the given bill.
func CreateTransaction(receiverPubKey []byte, k *account.AccountKey, amount uint64, systemID []byte, b *wallet.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	if b.Value <= amount {
		return NewTransferTx(receiverPubKey, k, systemID, b, timeout, fcrID)
	}
	targetUnits := []*money.TargetUnit{
		{Amount: amount, OwnerCondition: script.PredicatePayToPublicKeyHashDefault(hash.Sum256(receiverPubKey))},
	}
	remainingValue := b.Value - amount
	return NewSplitTx(targetUnits, remainingValue, k, systemID, b, timeout, fcrID)
}

// NewTransferTx creates a P2PKH transfer transaction.
func NewTransferTx(receiverPubKey []byte, k *account.AccountKey, systemID []byte, bill *wallet.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(receiverPubKey)),
		TargetValue: bill.Value,
		Backlink:    bill.TxHash,
	}
	txPayload, err := newTxPayload(systemID, money.PayloadTypeTransfer, bill.GetID(), timeout, fcrID, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, k)
}

// NewSplitTx creates a P2PKH split transaction.
func NewSplitTx(targetUnits []*money.TargetUnit, remainingValue uint64, k *account.AccountKey, systemID []byte, bill *wallet.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	attr := &money.SplitAttributes{
		TargetUnits:    targetUnits,
		RemainingValue: remainingValue,
		Backlink:       bill.TxHash,
	}
	txPayload, err := newTxPayload(systemID, money.PayloadTypeSplit, bill.GetID(), timeout, fcrID, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, k)
}

func NewDustTx(ac *account.AccountKey, systemID []byte, bill *wallet.Bill, targetBill *wallet.Bill, timeout uint64) (*types.TransactionOrder, error) {
	attr := &money.TransferDCAttributes{
		TargetUnitID:       targetBill.Id,
		Value:              bill.Value,
		TargetUnitBacklink: targetBill.TxHash,
		Backlink:           bill.TxHash,
	}
	txPayload, err := newTxPayload(systemID, money.PayloadTypeTransDC, bill.GetID(), timeout, money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256), attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, ac)
}

func NewSwapTx(k *account.AccountKey, systemID []byte, dcProofs []*wallet.Proof, targetUnitID []byte, timeout uint64) (*types.TransactionOrder, error) {
	if len(dcProofs) == 0 {
		return nil, errors.New("cannot create swap transaction as no dust transfer proofs exist")
	}
	// sort proofs by ids smallest first
	sort.Slice(dcProofs, func(i, j int) bool {
		return bytes.Compare(dcProofs[i].TxRecord.TransactionOrder.UnitID(), dcProofs[j].TxRecord.TransactionOrder.UnitID()) < 0
	})
	var dustTransferProofs []*types.TxProof
	var dustTransferRecords []*types.TransactionRecord
	var billValueSum uint64
	for _, p := range dcProofs {
		dustTransferRecords = append(dustTransferRecords, p.TxRecord)
		dustTransferProofs = append(dustTransferProofs, p.TxProof)
		var attr *money.TransferDCAttributes
		if err := p.TxRecord.TransactionOrder.UnmarshalAttributes(&attr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal dust transfer tx: %w", err)
		}
		billValueSum += attr.Value
	}
	attr := &money.SwapDCAttributes{
		OwnerCondition:   script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256),
		DcTransfers:      dustTransferRecords,
		DcTransferProofs: dustTransferProofs,
		TargetValue:      billValueSum,
	}
	swapTx, err := newTxPayload(systemID, money.PayloadTypeSwapDC, targetUnitID, timeout, money.NewFeeCreditRecordID(nil, k.PubKeyHash.Sha256), attr)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap transaction: %w", err)
	}
	payload, err := signPayload(swapTx, k)
	if err != nil {
		return nil, fmt.Errorf("failed to sign swap transaction: %w", err)
	}
	return payload, nil
}

func NewTransferFCTx(amount uint64, targetRecordID []byte, targetUnitBacklink []byte, k *account.AccountKey, moneySystemID, targetSystemID []byte, unit *wallet.Bill, timeout, t1, t2 uint64) (*types.TransactionOrder, error) {
	attr := &transactions.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: targetSystemID,
		TargetRecordID:         targetRecordID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
		TargetUnitBacklink:     targetUnitBacklink,
		Backlink:               unit.TxHash,
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

func NewCloseFCTx(systemID []byte, unitID []byte, timeout uint64, amount uint64, targetUnitID, targetUnitBacklink []byte, k *account.AccountKey) (*types.TransactionOrder, error) {
	attr := &transactions.CloseFeeCreditAttributes{
		Amount:             amount,
		TargetUnitID:       targetUnitID,
		TargetUnitBacklink: targetUnitBacklink,
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
