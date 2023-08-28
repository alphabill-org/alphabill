package money

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
)

const (
	txTimeoutBlockCount       = 10
	maxBillsForDustCollection = 100
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance for transaction")
	ErrInvalidPubKey       = errors.New("invalid public key, public key must be in compressed secp256k1 format")

	ErrNoFeeCredit           = errors.New("no fee credit in money wallet")
	ErrInsufficientFeeCredit = errors.New("insufficient fee credit balance for transaction(s)")
)

type (
	Wallet struct {
		am            account.Manager
		backend       BackendAPI
		feeManager    *fees.FeeManager
		TxPublisher   *TxPublisher
		unitLocker    UnitLocker
		dustCollector *DustCollector
	}

	BackendAPI interface {
		GetBalance(ctx context.Context, pubKey []byte, includeDCBills bool) (uint64, error)
		ListBills(ctx context.Context, pubKey []byte, includeDCBills bool, offsetKey string, limit int) (*backend.ListBillsResponse, error)
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

	DustCollectionResult struct {
		AccountNumber uint64
		SwapProof     *wallet.Proof
	}
)

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
// If mnemonic seed is empty then new mnemonic will ge generated, otherwise wallet is restored using given mnemonic.
func CreateNewWallet(am account.Manager, mnemonic string) error {
	return createMoneyWallet(mnemonic, am)
}

func LoadExistingWallet(am account.Manager, unitLocker UnitLocker, backend BackendAPI) (*Wallet, error) {
	moneySystemID := money.DefaultSystemIdentifier
	moneyTxPublisher := NewTxPublisher(backend)
	feeManager := fees.NewFeeManager(am, unitLocker, moneySystemID, moneyTxPublisher, backend, moneySystemID, moneyTxPublisher, backend)
	dustCollector := NewDustCollector(moneySystemID, maxBillsForDustCollection, backend, unitLocker)
	return &Wallet{
		am:            am,
		backend:       backend,
		TxPublisher:   moneyTxPublisher,
		feeManager:    feeManager,
		unitLocker:    unitLocker,
		dustCollector: dustCollector,
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
			swapProof, err := w.dustCollector.CollectDust(ctx, accKey)
			if err != nil {
				return nil, fmt.Errorf("dust collection failed for account number %d: %w", acc.AccountIndex+1, err)
			}
			res = append(res, &DustCollectionResult{
				AccountNumber: acc.AccountIndex + 1,
				SwapProof:     swapProof,
			})
		}
	} else {
		accKey, err := w.am.GetAccountKey(accountNumber - 1)
		if err != nil {
			return nil, fmt.Errorf("failed to load account key: %w", err)
		}
		swapProof, err := w.dustCollector.CollectDust(ctx, accKey)
		if err != nil {
			return nil, fmt.Errorf("dust collection failed for account number %d: %w", accountNumber, err)
		}
		res = append(res, &DustCollectionResult{
			AccountNumber: accountNumber,
			SwapProof:     swapProof,
		})
	}
	return res, nil
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

	bills, err := w.getUnlockedBills(ctx, pubKey)
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
	return w.GetFeeCreditBill(ctx, accountKey.PubKeyHash.Sha256)
}

// GetFeeCreditBill returns fee credit bill for given unitID
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*wallet.Bill, error) {
	return w.backend.GetFeeCreditBill(ctx, unitID)
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *Wallet) SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error) {
	return w.TxPublisher.SendTx(ctx, tx, senderPubKey)
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
	if len(c.ReceiverPubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
		return ErrInvalidPubKey
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
