package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"
	"time"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	dcTimeoutBlockCount       = 10
	swapTimeoutBlockCount     = 60
	txTimeoutBlockCount       = 10
	maxBillsForDustCollection = 100
	dustBillDeletionTimeout   = 65536
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance for transaction")
	ErrInvalidPubKey       = errors.New("invalid public key, public key must be in compressed secp256k1 format")

	ErrNoFeeCredit                  = errors.New("no fee credit in money wallet")
	ErrInsufficientFeeCredit        = errors.New("insufficient fee credit balance for transaction(s)")
	ErrInvalidCreateFeeCreditAmount = errors.New("fee credit amount must be positive")
)

type (
	Wallet struct {
		am          account.Manager
		backend     BackendAPI
		feeManager  *fees.FeeManager
		TxPublisher *TxPublisher
	}

	BackendAPI interface {
		GetBalance(pubKey []byte, includeDCBills bool) (uint64, error)
		ListBills(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error)
		GetBills(pubKey []byte) ([]*wallet.Bill, error)
		GetProof(billId []byte) (*wallet.Bills, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*wallet.Bill, error)
		PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error
		GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	}

	SendCmd struct {
		ReceiverPubKey      []byte
		Amount              uint64
		WaitForConfirmation bool
		AccountIndex        uint64
	}

	GetBalanceCmd struct {
		AccountIndex uint64
		CountDCBills bool
	}

	AddFeeCmd struct {
		Amount       uint64
		AccountIndex uint64
	}

	ReclaimFeeCmd struct {
		AccountIndex uint64
	}

	dcBillGroup struct {
		dcBills         []*Bill
		valueSum        uint64
		dcNonce         []byte
		dcTimeout       uint64
		billIdentifiers [][]byte
		dcSum           uint64
	}
)

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
// If mnemonic seed is empty then new mnemonic will ge generated, otherwise wallet is restored using given mnemonic.
func CreateNewWallet(am account.Manager, mnemonic string) error {
	return createMoneyWallet(mnemonic, am)
}

func LoadExistingWallet(am account.Manager, backend BackendAPI) (*Wallet, error) {
	moneySystemID := []byte{0, 0, 0, 0}
	moneyTxPublisher := NewTxPublisher(backend)
	feeManager := fees.NewFeeManager(am, moneySystemID, moneyTxPublisher, backend, moneySystemID, moneyTxPublisher, backend)
	return &Wallet{
		am:          am,
		backend:     backend,
		TxPublisher: moneyTxPublisher,
		feeManager:  feeManager,
	}, nil
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) SystemID() []byte {
	// TODO: return the default "AlphaBill Money System ID" for now
	// but this should come from config (base wallet? AB client?)
	return []byte{0, 0, 0, 0}
}

// Shutdown terminates connection to alphabill node, closes account manager and cancels any background goroutines.
func (w *Wallet) Shutdown() {
	w.am.Close()
}

// CollectDust starts the dust collector process for the requested accounts in the wallet.
// The function blocks until dust collector process is finished or timed out. Skips account if the account already has only one or no bills.
func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64) error {
	if accountNumber == 0 {
		for _, acc := range w.am.GetAll() {
			err := w.collectDust(ctx, acc.AccountIndex)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		return w.collectDust(ctx, accountNumber-1)
	}
}

// GetBalance returns sum value of all bills currently owned by the wallet, for given account.
// The value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalance(cmd GetBalanceCmd) (uint64, error) {
	pubKey, err := w.am.GetPublicKey(cmd.AccountIndex)
	if err != nil {
		return 0, err
	}
	return w.backend.GetBalance(pubKey, cmd.CountDCBills)
}

// GetBalances returns sum value of all bills currently owned by the wallet, for all accounts.
// The value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalances(cmd GetBalanceCmd) ([]uint64, uint64, error) {
	pubKeys, err := w.am.GetPublicKeys()
	totals := make([]uint64, len(pubKeys))
	sum := uint64(0)
	for accountIndex, pubKey := range pubKeys {
		balance, err := w.backend.GetBalance(pubKey, cmd.CountDCBills)
		if err != nil {
			return nil, 0, err
		}
		sum += balance
		totals[accountIndex] = balance
	}
	return totals, sum, err
}

// GetRoundNumber returns latest round number known to the wallet, including empty rounds.
func (w *Wallet) GetRoundNumber(ctx context.Context) (uint64, error) {
	return w.backend.GetRoundNumber(ctx)
}

// Send creates, signs and broadcasts transactions, in total for the given amount,
// to the given public key, the public key must be in compressed secp256k1 format.
// Sends one transaction per bill, prioritizing larger bills.
// Waits for initial response from the node, returns error if any transaction was not accepted to the mempool.
// Returns list of tx proofs, if waitForConfirmation=true, otherwise nil.
func (w *Wallet) Send(ctx context.Context, cmd SendCmd) ([]*wallet.Proof, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}

	pubKey, _ := w.am.GetPublicKey(cmd.AccountIndex)
	balance, err := w.backend.GetBalance(pubKey, true)
	if err != nil {
		return nil, err
	}
	if cmd.Amount > balance {
		return nil, ErrInsufficientBalance
	}

	roundNumber, err := w.backend.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	k, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	fcb, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
		return nil, err
	}
	if fcb == nil {
		return nil, ErrNoFeeCredit
	}

	bills, err := w.backend.GetBills(pubKey)
	if err != nil {
		return nil, err
	}

	timeout := roundNumber + txTimeoutBlockCount
	batch := txsubmitter.NewBatch(k.PubKey, w.backend)
	txs, err := tx_builder.CreateTransactions(cmd.ReceiverPubKey, cmd.Amount, w.SystemID(), bills, k, timeout, fcb.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactions: %w", err)
	}

	txsCost := tx_builder.MaxFee * uint64(len(batch.Submissions()))
	if fcb.Value < txsCost {
		return nil, ErrInsufficientFeeCredit
	}

	for _, tx := range txs {
		batch.Add(&txsubmitter.TxSubmission{
			UnitID:      tx.UnitID(),
			TxHash:      tx.Hash(crypto.SHA256),
			Transaction: tx,
		})
	}
	if err = batch.SendTx(ctx, cmd.WaitForConfirmation); err != nil {
		return nil, err
	}

	var proofs []*wallet.Proof
	for _, txSub := range batch.Submissions() {
		proofs = append(proofs, txSub.Proof)
	}
	return proofs, nil
}

func (w *Wallet) PostTransactions(ctx context.Context, pubkey wallet.PubKey, txs *wallet.Transactions) error {
	return w.backend.PostTransactions(ctx, pubkey, txs)
}

func (w *Wallet) GetTxProof(_ context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	resp, err := w.backend.GetProof(unitID)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		// confirmation expects nil (not error) if there's no proof for the given tx hash (yet)
		return nil, nil
	}
	if len(resp.Bills) != 1 {
		return nil, fmt.Errorf("unexpected number of proofs: %d, bill ID: %X", len(resp.Bills), unitID)
	}
	bill := resp.Bills[0]
	if !bytes.Equal(bill.TxHash, txHash) {
		// confirmation expects nil (not error) if there's no proof for the given tx hash (yet)
		return nil, nil
	}
	return bill.TxProof, nil
}

// AddFeeCredit creates fee credit for the given amount.
// Wallet must have a bill large enough for the required amount plus fees.
// Returns transferFC and addFC transaction proofs.
func (w *Wallet) AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) ([]*wallet.Proof, error) {
	return w.feeManager.AddFeeCredit(ctx, cmd)
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns closeFC and reclaimFC transaction proofs.
func (w *Wallet) ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) ([]*wallet.Proof, error) {
	return w.feeManager.ReclaimFeeCredit(ctx, cmd)
}

// GetFeeCredit returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCredit(ctx context.Context, cmd fees.GetFeeCreditCmd) (*wallet.Bill, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	return w.GetFeeCreditBill(ctx, accountKey.PrivKeyHash)
}

// GetFeeCreditBill returns fee credit bill for given unitID
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*wallet.Bill, error) {
	return w.backend.GetFeeCreditBill(ctx, unitID)
}

// collectDust sends dust transfer for every bill for given account in wallet.
// Returns immediately without error if there's already 1 or 0 bills.
// Once the dust transfers get confirmed on the ledger then swap transfer is broadcast.
func (w *Wallet) collectDust(ctx context.Context, accountIndex uint64) error {
	log.Info("starting dust collection for account=", accountIndex)
	roundNr, err := w.backend.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	pubKey, err := w.am.GetPublicKey(accountIndex)
	if err != nil {
		return err
	}
	bills, err := w.getDetailedBillsList(pubKey)
	if err != nil {
		return err
	}
	if len(bills) < 2 {
		log.Info("Account ", accountIndex, " has less than 2 bills, skipping dust collection")
		return nil
	}
	dcBillGroups, err := groupDcBills(bills)
	if err != nil {
		return err
	}
	if len(dcBillGroups) > 0 {
		for _, v := range dcBillGroups {
			if roundNr < v.dcTimeout && v.valueSum < v.dcSum {
				log.Info("waiting for dc confirmation(s)...")
				for roundNr <= v.dcTimeout {
					select {
					case <-time.After(500 * time.Millisecond):
						roundNr, err = w.backend.GetRoundNumber(ctx)
						if err != nil {
							return err
						}
						continue
					case <-ctx.Done():
						return nil
					}
				}
				bills, err = w.getDetailedBillsList(pubKey)
				if err != nil {
					return err
				}
				dcBillGroups, err = groupDcBills(bills)
				if err != nil {
					return err
				}
				v = dcBillGroups[string(v.dcNonce)]
			}
			swapTimeout := roundNr + swapTimeoutBlockCount
			if err := w.swapDcBills(ctx, v.dcBills, v.dcNonce, v.billIdentifiers, swapTimeout, accountIndex); err != nil {
				return err
			}
		}
	} else {
		k, err := w.am.GetAccountKey(accountIndex)
		if err != nil {
			return err
		}
		billCount := len(bills)
		for billCount > 1 {
			offset := maxBillsForDustCollection
			if offset > billCount {
				offset = billCount
			}
			err = w.submitDCBatch(ctx, k, bills[:offset], roundNr, accountIndex)
			if err != nil {
				return err
			}
			bills, err = w.getDetailedBillsList(pubKey)
			if err != nil {
				return err
			}
			billCount = len(bills)
		}
	}

	log.Info("finished waiting for blocking collect dust on account=", accountIndex)

	return nil
}

func (w *Wallet) submitDCBatch(ctx context.Context, k *account.AccountKey, bills []*Bill, roundNr, accountIndex uint64) error {
	dcBatch := txsubmitter.NewBatch(k.PubKey, w.backend)
	dcTimeout := roundNr + dcTimeoutBlockCount
	dcNonce := calculateDcNonce(bills)
	billIds, dcSum := getBillIdsAndSum(bills)
	for _, b := range bills {
		tx, err := tx_builder.NewDustTx(k, w.SystemID(), &wallet.Bill{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}, dcNonce, billIds, dcTimeout, dcSum)
		if err != nil {
			return err
		}
		dcBatch.Add(&txsubmitter.TxSubmission{
			UnitID:      tx.UnitID(),
			TxHash:      tx.Hash(crypto.SHA256),
			Transaction: tx,
		})
	}
	if err := dcBatch.SendTx(ctx, true); err != nil {
		return err
	}

	swapTimeout := roundNr + swapTimeoutBlockCount
	for _, sub := range dcBatch.Submissions() {
		for _, b := range bills {
			if bytes.Equal(util.Uint256ToBytes(b.Id), sub.UnitID) {
				b.TxProof = sub.Proof
				break
			}
		}
	}

	return w.swapDcBills(ctx, bills, dcNonce, billIds, swapTimeout, accountIndex)
}

func (w *Wallet) swapDcBills(ctx context.Context, dcBills []*Bill, dcNonce []byte, billIds [][]byte, timeout, accountIndex uint64) error {
	k, err := w.am.GetAccountKey(accountIndex)
	if err != nil {
		return err
	}
	fcb, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: accountIndex})
	if err != nil {
		return err
	}
	if fcb.GetValue() < tx_builder.MaxFee {
		return ErrInsufficientFeeCredit
	}

	var bpBills []*wallet.Bill
	for _, b := range dcBills {
		bpBills = append(bpBills, &wallet.Bill{Id: b.GetID(), Value: b.Value, TxProof: b.TxProof})
	}
	swapTx, err := tx_builder.NewSwapTx(k, w.SystemID(), bpBills, dcNonce, billIds, timeout)
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("sending swap tx: nonce=%s timeout=%d", hexutil.Encode(dcNonce), timeout))
	dcBatch := txsubmitter.NewBatch(k.PubKey, w.backend)
	dcBatch.Add(&txsubmitter.TxSubmission{
		UnitID:      swapTx.UnitID(),
		TxHash:      swapTx.Hash(crypto.SHA256),
		Transaction: swapTx,
	})

	return dcBatch.SendTx(ctx, true)
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *Wallet) SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error) {
	return w.TxPublisher.SendTx(ctx, tx, senderPubKey)
}

func (w *Wallet) getDetailedBillsList(pubKey []byte) ([]*Bill, error) {
	billResponse, err := w.backend.ListBills(pubKey, true)
	if err != nil {
		return nil, err
	}
	bills := make([]*Bill, 0)
	if len(billResponse.Bills) < 2 {
		return bills, nil
	}
	for _, b := range billResponse.Bills {
		if b.IsDCBill {
			proof, err := w.backend.GetProof(b.Id)
			if err != nil {
				return nil, err
			}
			bills = append(bills, convertBill(proof.Bills[0]))
		} else {
			bills = append(bills, &Bill{Id: util.BytesToUint256(b.Id), Value: b.Value, TxHash: b.TxHash, IsDcBill: b.IsDCBill})
		}
	}

	return bills, nil
}

func (c *SendCmd) isValid() error {
	if len(c.ReceiverPubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
		return ErrInvalidPubKey
	}
	return nil
}

func (c *AddFeeCmd) isValid() error {
	if c.Amount == 0 {
		return ErrInvalidCreateFeeCreditAmount
	}
	return nil
}

func createMoneyWallet(mnemonic string, am account.Manager) error {
	// load accounts from account manager
	accountKeys, err := am.GetAccountKeys()
	if err != nil {
		return fmt.Errorf("failed to check does account have any keys: %w", err)
	}
	// create keys in account manager if not exists
	if len(accountKeys) == 0 {
		// creating keys also adds the first account
		if err = am.CreateKeys(mnemonic); err != nil {
			return fmt.Errorf("failed to create keys for the account: %w", err)
		}
		// reload accounts after adding the first account
		accountKeys, err = am.GetAccountKeys()
		if err != nil {
			return fmt.Errorf("failed to read account keys: %w", err)
		}
		if len(accountKeys) == 0 {
			return errors.New("failed to create key for the first account")
		}
	}

	return nil
}

func calculateDcNonce(bills []*Bill) []byte {
	billIds, _ := getBillIdsAndSum(bills)

	// sort billIds in ascending order
	sort.Slice(billIds, func(i, j int) bool {
		return bytes.Compare(billIds[i], billIds[j]) < 0
	})

	hasher := crypto.SHA256.New()
	for _, billId := range billIds {
		hasher.Write(billId)
	}
	return hasher.Sum(nil)
}

// groupDcBills groups bills together by dc nonce
func groupDcBills(bills []*Bill) (map[string]*dcBillGroup, error) {
	m := map[string]*dcBillGroup{}
	for _, b := range bills {
		if b.IsDcBill {
			k := string(b.DcNonce)
			billContainer, exists := m[k]
			if !exists {
				billContainer = &dcBillGroup{}
				m[k] = billContainer
			}
			a := &money.TransferDCAttributes{}
			if err := b.TxProof.TxRecord.TransactionOrder.UnmarshalAttributes(a); err != nil {
				return nil, fmt.Errorf("invalid DC transfer: %w", err)
			}
			billContainer.dcSum = a.DCMetadata.DCSum
			billContainer.billIdentifiers = a.DCMetadata.BillIdentifiers
			billContainer.valueSum += b.Value
			billContainer.dcBills = append(billContainer.dcBills, b)
			billContainer.dcNonce = b.DcNonce
			billContainer.dcTimeout = b.DcTimeout
		}
	}
	return m, nil
}

func getBillIdsAndSum(bills []*Bill) ([][]byte, uint64) {
	var billIds [][]byte
	var sum uint64
	for _, b := range bills {
		sum += b.Value
		billIds = append(billIds, b.GetID())
	}
	return billIds, sum
}

// converts proto wallet.Bill to money.Bill domain struct
func convertBill(b *wallet.Bill) *Bill {
	if b.IsDcBill {
		attrs := &money.TransferDCAttributes{}
		if err := b.TxProof.TxRecord.TransactionOrder.UnmarshalAttributes(attrs); err != nil {
			return nil
		}
		return &Bill{
			Id:        util.BytesToUint256(b.Id),
			Value:     b.Value,
			TxHash:    b.TxHash,
			IsDcBill:  b.IsDcBill,
			DcNonce:   attrs.Nonce,
			DcTimeout: b.TxProof.TxRecord.TransactionOrder.Timeout(),
			TxProof:   b.TxProof,
		}
	}
	return &Bill{
		Id:       util.BytesToUint256(b.Id),
		Value:    b.Value,
		TxHash:   b.TxHash,
		IsDcBill: b.IsDcBill,
		TxProof:  b.TxProof,
	}
}

// NewFeeManager helper struct for creating new money partition fee manager
func NewFeeManager(am account.Manager, systemID []byte, moneyClient BackendAPI) *fees.FeeManager {
	moneyTxPublisher := NewTxPublisher(moneyClient)
	return fees.NewFeeManager(am, systemID, moneyTxPublisher, moneyClient, systemID, moneyTxPublisher, moneyClient)
}
