package tx_builder

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	money2 "github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/validator/internal/predicates/templates"
	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/account"
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
	targetUnits := []*money2.TargetUnit{
		{Amount: amount, OwnerCondition: templates.NewP2pkh256BytesFromKey(receiverPubKey)},
	}
	remainingValue := b.Value - amount
	return NewSplitTx(targetUnits, remainingValue, k, systemID, b, timeout, fcrID)
}

// NewTransferTx creates a P2PKH transfer transaction.
func NewTransferTx(receiverPubKey []byte, k *account.AccountKey, systemID []byte, bill *wallet.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	attr := &money2.TransferAttributes{
		NewBearer:   templates.NewP2pkh256BytesFromKey(receiverPubKey),
		TargetValue: bill.Value,
		Backlink:    bill.TxHash,
	}
	txPayload, err := newTxPayload(systemID, money2.PayloadTypeTransfer, bill.GetID(), timeout, fcrID, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, k)
}

// NewSplitTx creates a P2PKH split transaction.
func NewSplitTx(targetUnits []*money2.TargetUnit, remainingValue uint64, k *account.AccountKey, systemID []byte, bill *wallet.Bill, timeout uint64, fcrID []byte) (*types.TransactionOrder, error) {
	attr := &money2.SplitAttributes{
		TargetUnits:    targetUnits,
		RemainingValue: remainingValue,
		Backlink:       bill.TxHash,
	}
	txPayload, err := newTxPayload(systemID, money2.PayloadTypeSplit, bill.GetID(), timeout, fcrID, attr)
	if err != nil {
		return nil, err
	}
	return signPayload(txPayload, k)
}

func NewDustTx(ac *account.AccountKey, systemID []byte, bill *wallet.Bill, targetBill *wallet.Bill, timeout uint64) (*types.TransactionOrder, error) {
	attr := &money2.TransferDCAttributes{
		TargetUnitID:       targetBill.Id,
		Value:              bill.Value,
		TargetUnitBacklink: targetBill.TxHash,
		Backlink:           bill.TxHash,
	}
	txPayload, err := newTxPayload(systemID, money2.PayloadTypeTransDC, bill.GetID(), timeout, money2.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256), attr)
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
		var attr *money2.TransferDCAttributes
		if err := p.TxRecord.TransactionOrder.UnmarshalAttributes(&attr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal dust transfer tx: %w", err)
		}
		billValueSum += attr.Value
	}
	attr := &money2.SwapDCAttributes{
		OwnerCondition:   templates.NewP2pkh256BytesFromKeyHash(k.PubKeyHash.Sha256),
		DcTransfers:      dustTransferRecords,
		DcTransferProofs: dustTransferProofs,
		TargetValue:      billValueSum,
	}
	swapTx, err := newTxPayload(systemID, money2.PayloadTypeSwapDC, targetUnitID, timeout, money2.NewFeeCreditRecordID(nil, k.PubKeyHash.Sha256), attr)
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
		FeeCreditOwnerCondition: templates.NewP2pkh256BytesFromKeyHash(ac.PubKeyHash.Sha256),
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
		OwnerProof: templates.NewP2pkh256SignatureBytes(sig, ac.PubKey),
	}, nil
}
