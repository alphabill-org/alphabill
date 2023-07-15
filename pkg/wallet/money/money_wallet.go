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
	swapTimeoutBlockCount     = 60 // swap timeout (after which dust bills can be deleted)
	swapTxTimeoutBlockCount   = 60 // swap tx message timeout (client metadata)
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
		GetBalance(ctx context.Context, pubKey []byte, includeDCBills bool) (uint64, error)
		ListBills(ctx context.Context, pubKey []byte, includeDCBills, includeDCMetadata bool) (*backend.ListBillsResponse, error)
		GetBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*wallet.Bill, error)
		GetLockedFeeCredit(ctx context.Context, systemID []byte, unitID []byte) (*types.TransactionRecord, error)
		GetClosedFeeCredit(ctx context.Context, fcbID []byte) (*types.TransactionRecord, error)
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
		dcBills     []*Bill
		valueSum    uint64
		dcNonce     []byte
		dcTimeout   uint64
		swapTimeout uint64
	}
)

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
// If mnemonic seed is empty then new mnemonic will ge generated, otherwise wallet is restored using given mnemonic.
func CreateNewWallet(am account.Manager, mnemonic string) error {
	return createMoneyWallet(mnemonic, am)
}

func LoadExistingWallet(am account.Manager, backend BackendAPI) (*Wallet, error) {
	moneySystemID := money.DefaultSystemIdentifier
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
	return money.DefaultSystemIdentifier
}

// Close terminates connection to alphabill node, closes account manager and cancels any background goroutines.
func (w *Wallet) Close() {
	w.am.Close()
	w.feeManager.Close()
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
func (w *Wallet) GetBalance(ctx context.Context, cmd GetBalanceCmd) (uint64, error) {
	pubKey, err := w.am.GetPublicKey(cmd.AccountIndex)
	if err != nil {
		return 0, err
	}
	return w.backend.GetBalance(ctx, pubKey, cmd.CountDCBills)
}

// GetBalances returns sum value of all bills currently owned by the wallet, for all accounts.
// The value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalances(ctx context.Context, cmd GetBalanceCmd) ([]uint64, uint64, error) {
	pubKeys, err := w.am.GetPublicKeys()
	totals := make([]uint64, len(pubKeys))
	sum := uint64(0)
	for accountIndex, pubKey := range pubKeys {
		balance, err := w.backend.GetBalance(ctx, pubKey, cmd.CountDCBills)
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
	balance, err := w.backend.GetBalance(ctx, pubKey, true)
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

	bills, err := w.backend.GetBills(ctx, pubKey)
	if err != nil {
		return nil, err
	}

	timeout := roundNumber + txTimeoutBlockCount
	batch := txsubmitter.NewBatch(k.PubKey, w.backend)
	txs, err := tx_builder.CreateTransactions(cmd.ReceiverPubKey, cmd.Amount, w.SystemID(), bills, k, timeout, fcb.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactions: %w", err)
	}

	for _, tx := range txs {
		batch.Add(&txsubmitter.TxSubmission{
			UnitID:      tx.UnitID(),
			TxHash:      tx.Hash(crypto.SHA256),
			Transaction: tx,
		})
	}

	txsCost := tx_builder.MaxFee * uint64(len(batch.Submissions()))
	if fcb.Value < txsCost {
		return nil, ErrInsufficientFeeCredit
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

// AddFeeCredit creates fee credit for the given amount.
// Wallet must have a bill large enough for the required amount plus fees.
// Returns transferFC and addFC transaction proofs.
func (w *Wallet) AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) (*fees.AddFeeCmdResponse, error) {
	return w.feeManager.AddFeeCredit(ctx, cmd)
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns closeFC and reclaimFC transaction proofs.
func (w *Wallet) ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) (*fees.ReclaimFeeCmdResponse, error) {
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
	fcb, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: accountIndex})
	if err != nil {
		return err
	}
	if fcb == nil {
		return ErrNoFeeCredit
	}
	bills, dcMetadataMap, err := w.getDetailedBillsList(ctx, pubKey)
	if err != nil {
		return err
	}
	if len(bills) < 2 {
		log.Info("Account ", accountIndex, " has less than 2 bills, skipping dust collection")
		return nil
	}
	dcBillGroups, err := groupDcBills(bills, roundNr)
	if err != nil {
		return err
	}
	if len(dcBillGroups) > 0 {
		for _, v := range dcBillGroups {
			if roundNr < v.dcTimeout && v.valueSum < dcMetadataMap[string(v.dcNonce)].DCSum {
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
				bills, dcMetadataMap, err = w.getDetailedBillsList(ctx, pubKey)
				if err != nil {
					return err
				}
				dcBillGroups, err = groupDcBills(bills, roundNr)
				if err != nil {
					return err
				}
				v = dcBillGroups[string(v.dcNonce)]
			}
			if fcb.GetValue() < tx_builder.MaxFee {
				return ErrInsufficientFeeCredit
			}
			swapTimeout := util.Min(roundNr+swapTxTimeoutBlockCount, v.swapTimeout)
			if err := w.swapDcBills(ctx, v.dcBills, v.dcNonce, dcMetadataMap[string(v.dcNonce)].BillIdentifiers, swapTimeout, accountIndex); err != nil {
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
			offset := util.Min(billCount, maxBillsForDustCollection)
			err = w.submitDCBatch(ctx, k, bills[:offset], fcb.Value, roundNr, accountIndex)
			if err != nil {
				return err
			}
			bills, _, err = w.getDetailedBillsList(ctx, pubKey)
			if err != nil {
				return err
			}
			billCount = len(bills)
		}
	}

	log.Info("finished waiting for blocking collect dust on account=", accountIndex)

	return nil
}

func (w *Wallet) submitDCBatch(ctx context.Context, k *account.AccountKey, bills []*Bill, fcbValue, roundNr, accountIndex uint64) error {
	dcBatch := txsubmitter.NewBatch(k.PubKey, w.backend)
	dcTimeout := roundNr + dcTimeoutBlockCount
	swapTimeout := roundNr + swapTimeoutBlockCount
	swapTxTimeout := roundNr + swapTxTimeoutBlockCount
	dcNonce := calculateDcNonce(bills)
	for _, b := range bills {
		tx, err := tx_builder.NewDustTx(k, w.SystemID(), &wallet.Bill{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash, SwapTimeout: swapTimeout}, dcNonce, dcTimeout)
		if err != nil {
			return err
		}
		dcBatch.Add(&txsubmitter.TxSubmission{
			UnitID:      tx.UnitID(),
			TxHash:      tx.Hash(crypto.SHA256),
			Transaction: tx,
		})
	}

	txsCost := tx_builder.MaxFee * uint64(len(dcBatch.Submissions()))
	if fcbValue < txsCost {
		return ErrInsufficientFeeCredit
	}

	if err := dcBatch.SendTx(ctx, true); err != nil {
		return err
	}

	for _, sub := range dcBatch.Submissions() {
		for _, b := range bills {
			if bytes.Equal(util.Uint256ToBytes(b.Id), sub.UnitID) {
				b.TxProof = sub.Proof
				break
			}
		}
	}

	return w.swapDcBills(ctx, bills, dcNonce, getBillIds(bills), swapTxTimeout, accountIndex)
}

func (w *Wallet) swapDcBills(ctx context.Context, dcBills []*Bill, dcNonce []byte, billIds [][]byte, timeout, accountIndex uint64) error {
	k, err := w.am.GetAccountKey(accountIndex)
	if err != nil {
		return err
	}
	var bpBills []*wallet.BillProof
	for _, b := range dcBills {
		bpBills = append(bpBills, &wallet.BillProof{Bill: &wallet.Bill{Id: b.GetID(), Value: b.Value}, TxProof: b.TxProof})
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

func (w *Wallet) getDetailedBillsList(ctx context.Context, pubKey []byte) ([]*Bill, map[string]*backend.DCMetadata, error) {
	billResponse, err := w.backend.ListBills(ctx, pubKey, true, true)
	if err != nil {
		return nil, nil, err
	}
	bills := make([]*Bill, 0)
	if len(billResponse.Bills) < 2 {
		return bills, nil, nil
	}
	for _, b := range billResponse.Bills {
		var proof *wallet.Proof
		if b.DcNonce != nil {
			proof, err = w.backend.GetTxProof(ctx, b.Id, b.TxHash)
			if err != nil {
				return nil, nil, err
			}
		}
		bills = append(bills, &Bill{Id: util.BytesToUint256(b.Id), Value: b.Value, TxHash: b.TxHash, DcNonce: b.DcNonce, SwapTimeout: b.SwapTimeout, TxProof: proof})
	}

	return bills, billResponse.DCMetadata, nil
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
	billIds := getBillIds(bills)

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
func groupDcBills(bills []*Bill, roundNumber uint64) (map[string]*dcBillGroup, error) {
	m := map[string]*dcBillGroup{}
	for _, b := range bills {
		if b.SwapTimeout > 0 && b.SwapTimeout <= roundNumber {
			continue // node and backend do not currently delete expired dc bills, so we just ignore them
		}
		if b.DcNonce != nil {
			k := string(b.DcNonce)
			billContainer, exists := m[k]
			if !exists {
				billContainer = &dcBillGroup{}
				m[k] = billContainer
			}
			billContainer.valueSum += b.Value
			billContainer.dcBills = append(billContainer.dcBills, b)
			billContainer.dcNonce = b.DcNonce
			billContainer.dcTimeout = b.DcTimeout
			billContainer.swapTimeout = b.SwapTimeout
		}
	}
	return m, nil
}

func getBillIds(bills []*Bill) [][]byte {
	var billIds [][]byte
	var sum uint64
	for _, b := range bills {
		sum += b.Value
		billIds = append(billIds, b.GetID())
	}
	return billIds
}
