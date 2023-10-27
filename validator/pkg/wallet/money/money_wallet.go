package money

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/alphabill-org/alphabill/api/predicates/templates"
	"github.com/alphabill-org/alphabill/api/types"
	abcrypto "github.com/alphabill-org/alphabill/common/crypto"
	"github.com/alphabill-org/alphabill/common/hash"
	money2 "github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/money/backend"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/txsubmitter"
)

const (
	txTimeoutBlockCount       = 10
	maxBillsForDustCollection = 100
)

type (
	Wallet struct {
		am            account.Manager
		backend       BackendAPI
		feeManager    *fees.FeeManager
		TxPublisher   *TxPublisher
		unitLocker    UnitLocker
		dustCollector *DustCollector
		log           *slog.Logger
	}

	BackendAPI interface {
		GetBalance(ctx context.Context, pubKey []byte, includeDCBills bool) (uint64, error)
		ListBills(ctx context.Context, pubKey []byte, includeDCBills bool, offsetKey string, limit int) (*backend.ListBillsResponse, error)
		GetBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error)
		GetLockedFeeCredit(ctx context.Context, systemID []byte, unitID []byte) (*types.TransactionRecord, error)
		GetClosedFeeCredit(ctx context.Context, fcbID []byte) (*types.TransactionRecord, error)
		PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error
		GetTxProof(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	}

	SendCmd struct {
		Receivers           []ReceiverData
		WaitForConfirmation bool
		AccountIndex        uint64
	}

	ReceiverData struct {
		PubKey []byte
		Amount uint64
	}

	GetBalanceCmd struct {
		AccountIndex uint64
		CountDCBills bool
	}
)

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
// If mnemonic seed is empty then new mnemonic will ge generated, otherwise wallet is restored using given mnemonic.
func CreateNewWallet(am account.Manager, mnemonic string) error {
	return createMoneyWallet(mnemonic, am)
}

func LoadExistingWallet(am account.Manager, unitLocker UnitLocker, backend BackendAPI, log *slog.Logger) (*Wallet, error) {
	moneySystemID := money2.DefaultSystemIdentifier
	moneyTxPublisher := NewTxPublisher(backend, log)
	feeManager := fees.NewFeeManager(am, unitLocker, moneySystemID, moneyTxPublisher, backend, moneySystemID, moneyTxPublisher, backend, FeeCreditRecordIDFormPublicKey, log)
	dustCollector := NewDustCollector(moneySystemID, maxBillsForDustCollection, backend, unitLocker, log)
	return &Wallet{
		am:            am,
		backend:       backend,
		TxPublisher:   moneyTxPublisher,
		feeManager:    feeManager,
		unitLocker:    unitLocker,
		dustCollector: dustCollector,
		log:           log,
	}, nil
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) SystemID() []byte {
	// TODO: return the default "AlphaBill Money System ID" for now
	// but this should come from config (base wallet? AB client?)
	return money2.DefaultSystemIdentifier
}

// Close terminates connection to alphabill node, closes account manager and cancels any background goroutines.
func (w *Wallet) Close() {
	w.am.Close()
	w.feeManager.Close()
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

	pubKey, err := w.am.GetPublicKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}
	balance, err := w.backend.GetBalance(ctx, pubKey, true)
	if err != nil {
		return nil, err
	}
	totalAmount := cmd.totalAmount()
	if totalAmount > balance {
		return nil, errors.New("insufficient balance for transaction")
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
		return nil, errors.New("no fee credit in money wallet")
	}

	bills, err := w.getUnlockedBills(ctx, pubKey)
	if err != nil {
		return nil, err
	}
	timeout := roundNumber + txTimeoutBlockCount
	batch := txsubmitter.NewBatch(k.PubKey, w.backend, w.log)

	var txs []*types.TransactionOrder
	if len(cmd.Receivers) > 1 {
		// if more than one receiver then perform transaction as N-way split and require sufficiently large bill
		largestBill := bills[0]
		if largestBill.Value <= totalAmount {
			return nil, fmt.Errorf("sending to multiple addresses is performed using a single N-way split "+
				"transaction which requires sufficiently large bill, need at least %d Tema value bill, have largest "+
				"%d Tema value bill", totalAmount+1, largestBill.Value) // +1 because 0 remaining value is not allowed
		}
		// convert send cmd targets to transaction units
		var targetUnits []*money2.TargetUnit
		for _, r := range cmd.Receivers {
			targetUnits = append(targetUnits, &money2.TargetUnit{
				Amount:         r.Amount,
				OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(r.PubKey)),
			})
		}
		remainingValue := largestBill.Value - totalAmount
		tx, err := tx_builder.NewSplitTx(targetUnits, remainingValue, k, w.SystemID(), largestBill, timeout, fcb.Id)
		if err != nil {
			return nil, fmt.Errorf("failed to create N-way split tx: %w", err)
		}
		txs = append(txs, tx)
	} else {
		// if single receiver then perform up to N transfers (until target amount is reached)
		txs, err = tx_builder.CreateTransactions(cmd.Receivers[0].PubKey, cmd.Receivers[0].Amount, w.SystemID(), bills, k, timeout, fcb.Id)
		if err != nil {
			return nil, fmt.Errorf("failed to create transactions: %w", err)
		}
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
		return nil, errors.New("insufficient fee credit balance for transaction(s)")
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

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *Wallet) SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error) {
	return w.TxPublisher.SendTx(ctx, tx, senderPubKey)
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
	return w.GetFeeCreditBill(ctx, money2.NewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256))
}

// GetFeeCreditBill returns fee credit bill for given unitID
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
	return w.backend.GetFeeCreditBill(ctx, unitID)
}

// CollectDust starts the dust collector process for the requested accounts in the wallet.
// Dust collection process joins up to N units into existing target unit, prioritizing small units first.
// The largest unit in wallet is selected as the target unit.
// If accountNumber is equal to 0 then dust collection is run for all accounts, returns list of swap tx proofs
// together with account numbers, the proof can be nil if swap tx was not sent e.g. if there's not enough bills to swap.
// If accountNumber is greater than 0 then dust collection is run only for the specific account, returns single swap tx
// proof, the proof can be nil e.g. if there's not enough bills to swap.
func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64) ([]*DustCollectionResult, error) {
	var res []*DustCollectionResult
	if accountNumber == 0 {
		for _, acc := range w.am.GetAll() {
			accKey, err := w.am.GetAccountKey(acc.AccountIndex)
			if err != nil {
				return nil, fmt.Errorf("failed to load account key: %w", err)
			}
			dcResult, err := w.dustCollector.CollectDust(ctx, accKey)
			if err != nil {
				return nil, fmt.Errorf("dust collection failed for account number %d: %w", acc.AccountIndex+1, err)
			}
			dcResult.AccountIndex = acc.AccountIndex
			res = append(res, dcResult)
		}
	} else {
		accKey, err := w.am.GetAccountKey(accountNumber - 1)
		if err != nil {
			return nil, fmt.Errorf("failed to load account key: %w", err)
		}
		dcResult, err := w.dustCollector.CollectDust(ctx, accKey)
		if err != nil {
			return nil, fmt.Errorf("dust collection failed for account number %d: %w", accountNumber, err)
		}
		dcResult.AccountIndex = accountNumber - 1
		res = append(res, dcResult)
	}
	return res, nil
}

func (w *Wallet) getUnlockedBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error) {
	var unlockedBills []*wallet.Bill
	bills, err := w.backend.GetBills(ctx, pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	// sort bills by value largest first
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})
	// filter locked bills
	for _, b := range bills {
		lockedUnit, err := w.unitLocker.GetUnit(pubKey, b.GetID())
		if err != nil {
			return nil, fmt.Errorf("failed to get locked bill: %w", err)
		}
		if lockedUnit == nil {
			unlockedBills = append(unlockedBills, b)
		}
	}
	return unlockedBills, nil
}

func (c *SendCmd) isValid() error {
	if len(c.Receivers) == 0 {
		return errors.New("receivers is empty")
	}
	for _, r := range c.Receivers {
		if len(r.PubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
			return fmt.Errorf("invalid public key: public key must be in compressed secp256k1 format: "+
				"got %d bytes, expected %d bytes for public key 0x%x", len(r.PubKey), abcrypto.CompressedSecp256K1PublicKeySize, r.PubKey)
		}
		if r.Amount == 0 {
			return errors.New("invalid amount: amount must be greater than zero")
		}
	}
	return nil
}

func (c *SendCmd) totalAmount() uint64 {
	var sum uint64
	for _, r := range c.Receivers {
		sum += r.Amount
	}
	return sum
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
