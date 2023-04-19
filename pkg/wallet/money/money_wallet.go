package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"golang.org/x/sync/errgroup"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	abclient "github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	backendmoney "github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
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
	ErrInvalidPassword     = errors.New("invalid password")
	ErrTxFailedToConfirm   = errors.New("transaction(s) failed to confirm")

	ErrNoFeeCredit                  = errors.New("no fee credit in wallet")
	ErrInsufficientFeeCredit        = errors.New("insufficient fee credit balance for transaction(s)")
	ErrInsufficientBillValue        = errors.New("wallet does not have a bill large enough for fee transfer")
	ErrInvalidCreateFeeCreditAmount = errors.New("fee credit amount must be positive")
)

var (
	txBufferFullErrMsg = "tx buffer is full"
	maxTxFailedTries   = 3
)

type (
	Wallet struct {
		*wallet.Wallet

		dcWg       *dcWaitGroup
		am         account.Manager
		restClient *client.MoneyBackendClient
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
)

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
// If mnemonic seed is empty then new mnemonic will ge generated, otherwise wallet is restored using given mnemonic.
func CreateNewWallet(am account.Manager, mnemonic string) error {
	return createMoneyWallet(mnemonic, am)
}

func LoadExistingWallet(config abclient.AlphabillClientConfig, am account.Manager, restClient *client.MoneyBackendClient) (*Wallet, error) {
	mw := &Wallet{am: am, restClient: restClient, dcWg: newDcWaitGroup()}

	mw.Wallet = wallet.New().
		SetABClientConf(config).
		Build()

	return mw, nil
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
	w.Wallet.Shutdown()
	w.am.Close()
	if w.dcWg != nil {
		w.dcWg.ResetWaitGroup()
	}
}

// CollectDust starts the dust collector process for all accounts in the wallet.
// Wallet needs to be synchronizing using Sync or SyncToMaxBlockNumber in order to receive transactions and finish the process.
// The function blocks until dust collector process is finished or timed out. Skips account if the account already has only one or no bills.
func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64) error {
	errgrp, ctx := errgroup.WithContext(ctx)
	if accountNumber == 0 {
		for _, acc := range w.am.GetAll() {
			accIndex := acc.AccountIndex // copy value for closure
			errgrp.Go(func() error {
				return w.collectDust(ctx, true, accIndex)
			})
		}
	} else {
		errgrp.Go(func() error {
			return w.collectDust(ctx, true, accountNumber-1)
		})
	}
	return errgrp.Wait()
}

// GetBalance returns sum value of all bills currently owned by the wallet, for given account.
// The value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalance(cmd GetBalanceCmd) (uint64, error) {
	pubKey, err := w.am.GetPublicKey(cmd.AccountIndex)
	if err != nil {
		return 0, err
	}
	return w.restClient.GetBalance(pubKey, cmd.CountDCBills)
}

// GetBalances returns sum value of all bills currently owned by the wallet, for all accounts.
// The value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalances(cmd GetBalanceCmd) ([]uint64, uint64, error) {
	pubKeys, err := w.am.GetPublicKeys()
	totals := make([]uint64, len(pubKeys))
	sum := uint64(0)
	for accountIndex, pubKey := range pubKeys {
		balance, err := w.restClient.GetBalance(pubKey, cmd.CountDCBills)
		if err != nil {
			return nil, 0, err
		}
		sum += balance
		totals[accountIndex] = balance
	}
	return totals, sum, err
}

// Send creates, signs and broadcasts transactions, in total for the given amount,
// to the given public key, the public key must be in compressed secp256k1 format.
// Sends one transaction per bill, prioritzing larger bills.
// Waits for initial response from the node, returns error if any transaction was not accepted to the mempool.
// Returns list of bills including transaction and proof data, if waitForConfirmation=true, otherwise nil.
func (w *Wallet) Send(ctx context.Context, cmd SendCmd) ([]*Bill, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}

	pubKey, _ := w.am.GetPublicKey(cmd.AccountIndex)
	balance, err := w.restClient.GetBalance(pubKey, true)
	if err != nil {
		return nil, err
	}
	if cmd.Amount > balance {
		return nil, ErrInsufficientBalance
	}

	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	k, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	fcb, err := w.GetFeeCreditBill(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	if fcb == nil {
		log.Info("err no fee credit")
		return nil, ErrNoFeeCredit
	}

	billResponse, err := w.restClient.ListBills(pubKey)
	if err != nil {
		return nil, err
	}
	bills, err := convertBills(billResponse.Bills)
	if err != nil {
		return nil, err
	}

	timeout := roundNumber + txTimeoutBlockCount
	txs, err := createTransactions(cmd.ReceiverPubKey, cmd.Amount, w.SystemID(), bills, k, timeout, fcb.GetID())
	if err != nil {
		return nil, err
	}
	txsCost := maxFee * uint64(len(txs))
	if fcb.Value < txsCost {
		return nil, ErrInsufficientFeeCredit
	}

	for _, tx := range txs {
		log.Info("sending tx")
		err := w.SendTransaction(ctx, tx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
		if err != nil {
			return nil, err
		}
	}
	if cmd.WaitForConfirmation {
		txProofs, err := w.waitForConfirmation(ctx, txs, roundNumber, timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to wait for confirmation: %w", err)
		}
		txc := NewTxConverter(w.SystemID())
		var units []*Bill
		for _, proof := range txProofs {
			unit, err := newBill(proof, txc)
			if err != nil {
				return nil, err
			}
			units = append(units, unit)
		}
		return units, nil
	}
	return nil, nil
}

// AddFeeCredit creates fee credit for the given amount.
// Wallet must have a bill large enough for the required amount plus fees.
// Returns list of "add fee credit" transaction proofs.
func (w *Wallet) AddFeeCredit(ctx context.Context, cmd AddFeeCmd) ([]*BlockProof, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}
	balance, err := w.GetBalance(GetBalanceCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
		return nil, err
	}
	// must have enough balance for two txs (transferFC + addFC) and not end up with zero sum
	maxTotalFees := 2 * maxFee
	if cmd.Amount+maxTotalFees > balance {
		return nil, ErrInsufficientBalance
	}

	// fetch bills
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	listBillsResponse, err := w.restClient.ListBills(accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	// filter dc bills
	var bills []*Bill
	for _, b := range listBillsResponse.Bills {
		if !b.IsDCBill {
			bills = append(bills, newBillFromVM(b))
		}
	}
	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})

	// verify bill is large enough for required amount
	billToTransfer := bills[0]
	if billToTransfer.Value < cmd.Amount+maxTotalFees {
		return nil, ErrInsufficientBillValue
	}

	// fetch fee credit bill
	fcb, err := w.GetFeeCreditBill(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	roundNo, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	timeout := roundNo + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	log.Info("sending transfer fee credit transaction")
	tx, err := createTransferFCTx(cmd.Amount, accountKey.PrivKeyHash, fcb.GetTxHash(), accountKey, w.SystemID(), billToTransfer, roundNo, timeout)
	if err != nil {
		return nil, err
	}
	err = w.SendTransaction(ctx, tx, &wallet.SendOpts{})
	if err != nil {
		return nil, err
	}
	// TODO waiting can be canceled or AddFC tx can fail
	// record some metadata about transferFC so that AddFC can be retried
	fcTransferProof, err := w.waitForConfirmation(ctx, []*txsystem.Transaction{tx}, roundNo, timeout)
	if err != nil {
		return nil, err
	}

	log.Info("sending add fee credit transaction")
	addFCTx, err := createAddFCTx(accountKey.PrivKeyHash, fcTransferProof[0], accountKey, w.SystemID(), timeout)
	if err != nil {
		return nil, err
	}
	err = w.SendTransaction(ctx, addFCTx, &wallet.SendOpts{})
	if err != nil {
		return nil, err
	}
	return w.waitForConfirmation(ctx, []*txsystem.Transaction{addFCTx}, roundNo, timeout)
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns list of "reclaim fee credit" transaction proofs.
func (w *Wallet) ReclaimFeeCredit(ctx context.Context, cmd ReclaimFeeCmd) ([]*BlockProof, error) {
	k, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	roundNo, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	timeout := roundNo + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	fcb, err := w.GetFeeCreditBill(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	if fcb.GetValue() == 0 {
		return nil, ErrInsufficientFeeCredit
	}

	listBillsResponse, err := w.restClient.ListBills(k.PubKey)
	if err != nil {
		return nil, err
	}
	// filter dc bills
	var bills []*Bill
	for _, b := range listBillsResponse.Bills {
		if !b.IsDCBill {
			bills = append(bills, newBillFromVM(b))
		}
	}
	if len(bills) == 0 {
		return nil, errors.New("wallet must have a source bill to which to add reclaimed fee credits")
	}
	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})

	log.Info("sending close fee credit transaction")
	targetBill := bills[0]
	// TODO waiting can be canceled or ReclaimFC tx can fail
	// record some metadata about CloseFC so that ReclaimFC can be retried
	tx, err := createCloseFCTx(w.SystemID(), fcb.GetID(), timeout, fcb.Value, targetBill.GetID(), targetBill.TxHash, k)
	if err != nil {
		return nil, err
	}
	err = w.SendTransaction(ctx, tx, &wallet.SendOpts{})
	if err != nil {
		return nil, err
	}
	fcTransferProof, err := w.waitForConfirmation(ctx, []*txsystem.Transaction{tx}, roundNo, timeout)
	if err != nil {
		return nil, err
	}

	log.Info("sending reclaim fee credit transaction")
	addFCTx, err := createReclaimFCTx(w.SystemID(), targetBill.GetID(), timeout, fcTransferProof[0], targetBill.TxHash, k)
	if err != nil {
		return nil, err
	}
	err = w.SendTransaction(ctx, addFCTx, &wallet.SendOpts{})
	if err != nil {
		return nil, err
	}
	return w.waitForConfirmation(ctx, []*txsystem.Transaction{addFCTx}, roundNo, timeout)
}

// GetFeeCreditBill returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCreditBill(accountIndex uint64) (*Bill, error) {
	accountKey, err := w.am.GetAccountKey(accountIndex)
	if err != nil {
		return nil, err
	}
	fcb, err := w.restClient.GetFeeCreditBill(accountKey.PrivKeyHash)
	if err != nil {
		if errors.Is(err, client.ErrMissingFeeCreditBill) {
			return nil, nil
		}
		return nil, err
	}
	return convertBill(fcb), nil
}

// collectDust sends dust transfer for every bill for given account in wallet and records metadata.
// Returns immediately without error if there's already 1 or 0 bills.
// Once the dust transfers get confirmed on the ledger then swap transfer is broadcast and metadata cleared.
// If blocking is true then the function blocks until swap has been completed or timed out,
// if blocking is false then the function returns after sending the dc transfers.
func (w *Wallet) collectDust(ctx context.Context, blocking bool, accountIndex uint64) error {
	log.Info("starting dust collection for account=", accountIndex, " blocking=", blocking)
	roundNr, err := w.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	pubKey, err := w.am.GetPublicKey(accountIndex)
	if err != nil {
		return err
	}
	billResponse, err := w.restClient.ListBills(pubKey)
	if err != nil {
		return err
	}
	if len(billResponse.Bills) < 2 {
		log.Info("Account ", accountIndex, " has less than 2 bills, skipping dust collection")
		return nil
	}
	if len(billResponse.Bills) > maxBillsForDustCollection {
		log.Info("Account ", accountIndex, " has more than ", maxBillsForDustCollection, " bills, dust collection will take only the first ", maxBillsForDustCollection, " bills")
		billResponse.Bills = billResponse.Bills[0:maxBillsForDustCollection]
	}
	var bills []*Bill
	for _, b := range billResponse.Bills {
		proof, err := w.restClient.GetProof(b.Id)
		if err != nil {
			return err
		}
		bills = append(bills, convertBill(proof.Bills[0]))
	}
	var expectedSwaps []expectedSwap
	dcBillGroups := groupDcBills(bills)
	if len(dcBillGroups) > 0 {
		for _, v := range dcBillGroups {
			if roundNr >= v.dcTimeout {
				swapTimeout := roundNr + swapTimeoutBlockCount
				billIds := getBillIds(v.dcBills)
				err = w.swapDcBills(v.dcBills, v.dcNonce, billIds, swapTimeout, accountIndex)
				if err != nil {
					return err
				}
				expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: v.dcNonce, timeout: swapTimeout, dcSum: v.valueSum})
				w.dcWg.AddExpectedSwaps(expectedSwaps)
			} else {
				// expecting to receive swap during dcTimeout
				expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: v.dcNonce, timeout: v.dcTimeout, dcSum: v.valueSum})
				w.dcWg.AddExpectedSwaps(expectedSwaps)
				err = w.doSwap(ctx, accountIndex, v.dcTimeout)
				if err != nil {
					return err
				}
			}
		}
	} else {
		k, err := w.am.GetAccountKey(accountIndex)
		if err != nil {
			return err
		}

		dcTimeout := roundNr + dcTimeoutBlockCount
		dcNonce := calculateDcNonce(bills)
		var dcValueSum uint64
		for _, b := range bills {
			dcValueSum += b.Value
			tx, err := createDustTx(k, w.SystemID(), b, dcNonce, dcTimeout)
			if err != nil {
				return err
			}
			log.Info("sending dust transfer tx for bill=", b.Id, " account=", accountIndex)
			err = w.SendTransaction(ctx, tx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
			if err != nil {
				return err
			}
		}
		expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: dcNonce, timeout: dcTimeout, dcSum: dcValueSum})
		w.dcWg.AddExpectedSwaps(expectedSwaps)

		if err = w.doSwap(ctx, accountIndex, dcTimeout); err != nil {
			return err
		}
	}

	if blocking {
		if err = w.confirmSwap(ctx); err != nil {
			log.Error("failed to confirm swap tx", err)
			return err
		}
	}

	log.Info("finished waiting for blocking collect dust on account=", accountIndex)

	return nil
}

func (w *Wallet) doSwap(ctx context.Context, accountIndex, timeout uint64) error {
	roundNr, err := w.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	pubKey, err := w.am.GetPublicKey(accountIndex)
	if err != nil {
		return err
	}
	log.Info("waiting for swap confirmation(s)...")
	for roundNr <= timeout+1 {
		billResponse, err := w.restClient.ListBills(pubKey)
		if err != nil {
			return err
		}
		var bills []*Bill
		for _, b := range billResponse.Bills {
			proof, err := w.restClient.GetProof(b.Id)
			if err != nil {
				return err
			}
			bills = append(bills, convertBill(proof.Bills[0]))
		}
		dcBillGroups := groupDcBills(bills)
		if len(dcBillGroups) > 0 {
			for k, v := range dcBillGroups {
				s := w.dcWg.getExpectedSwap(k)
				if s.dcNonce != nil && roundNr >= v.dcTimeout || v.valueSum >= s.dcSum {
					swapTimeout := roundNr + swapTimeoutBlockCount
					billIds := getBillIds(v.dcBills)
					err = w.swapDcBills(v.dcBills, v.dcNonce, billIds, swapTimeout, accountIndex)
					if err != nil {
						return err
					}
					w.dcWg.UpdateTimeout(v.dcNonce, swapTimeout)
					return nil
				}
			}
		}
		select {
		case <-time.After(500 * time.Millisecond):
			roundNr, err = w.GetRoundNumber(ctx)
			if err != nil {
				return err
			}
			continue
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}

func (w *Wallet) confirmSwap(ctx context.Context) error {
	roundNr, err := w.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	log.Info("waiting for swap confirmation(s)...")
	swapTimeout := roundNr + swapTimeoutBlockCount
	for roundNr <= swapTimeout {
		if len(w.dcWg.swaps) == 0 {
			return nil
		}
		b, err := w.AlphabillClient.GetBlock(ctx, roundNr)
		if err != nil {
			return err
		}
		if b != nil {
			for _, tx := range b.Transactions {
				err = w.dcWg.DecrementSwaps(string(tx.UnitId))
				if err != nil {
					return err
				}
			}
			if len(w.dcWg.swaps) == 0 {
				return nil
			}
		}
		select {
		// wait for some time before retrying to fetch new block
		case <-time.After(500 * time.Millisecond):
			roundNr, err = w.GetRoundNumber(ctx)
			if err != nil {
				return err
			}
			continue
		case <-ctx.Done():
			return nil
		}
	}
	w.dcWg.ResetWaitGroup()

	return nil
}

func (w *Wallet) swapDcBills(dcBills []*Bill, dcNonce []byte, billIds [][]byte, timeout uint64, accountIndex uint64) error {
	k, err := w.am.GetAccountKey(accountIndex)
	if err != nil {
		return err
	}
	fcb, err := w.GetFeeCreditBill(accountIndex)
	if err != nil {
		return err
	}
	if fcb.GetValue() < maxFee {
		return ErrInsufficientFeeCredit
	}
	swap, err := createSwapTx(k, w.SystemID(), dcBills, dcNonce, billIds, timeout)
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("sending swap tx: nonce=%s timeout=%d", hexutil.Encode(dcNonce), timeout))
	err = w.SendTransaction(context.Background(), swap, &wallet.SendOpts{RetryOnFullTxBuffer: true})
	if err != nil {
		return err
	}
	return nil
}

func (w *Wallet) waitForConfirmation(ctx context.Context, pendingTxs []*txsystem.Transaction, latestRoundNumber, timeout uint64) ([]*BlockProof, error) {
	log.Info("waiting for confirmation(s)...")
	latestBlockNumber := latestRoundNumber
	txsLog := NewTxLog(pendingTxs)
	for latestBlockNumber <= timeout {
		b, err := w.AlphabillClient.GetBlock(ctx, latestBlockNumber)
		if err != nil {
			return nil, err
		}
		if b == nil {
			// block might be empty, check latest round number
			latestRoundNumber, err = w.AlphabillClient.GetRoundNumber(ctx)
			if err != nil {
				return nil, err
			}
			if latestRoundNumber > latestBlockNumber {
				latestBlockNumber++
				continue
			}
			// wait for some time before retrying to fetch new block
			select {
			case <-time.After(time.Second):
				continue
			case <-ctx.Done():
				return nil, nil
			}
		}

		// TODO no need to convert to generic tx?
		txc := NewTxConverter(w.SystemID())
		genericBlock, err := b.ToGenericBlock(txc)
		if err != nil {
			return nil, err
		}
		for _, gtx := range genericBlock.Transactions {
			tx := gtx.ToProtoBuf()
			if txsLog.Contains(tx) {
				log.Info("confirmed tx ", hexutil.Encode(tx.UnitId))
				err = txsLog.RecordTx(gtx, genericBlock)
				if err != nil {
					return nil, err
				}
				if txsLog.IsAllTxsConfirmed() {
					log.Info("transaction(s) confirmed")
					return txsLog.GetAllRecordedProofs(), nil
				}
			}
		}
		latestBlockNumber++
	}
	return nil, ErrTxFailedToConfirm
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
	var billIds [][]byte
	for _, b := range bills {
		billIds = append(billIds, b.GetID())
	}

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
func groupDcBills(bills []*Bill) map[string]*dcBillGroup {
	m := map[string]*dcBillGroup{}
	for _, b := range bills {
		if b.IsDcBill {
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
		}
	}
	return m
}

func hashId(id []byte) []byte {
	hasher := crypto.Hash.New(crypto.SHA256)
	hasher.Write(id)
	return hasher.Sum(nil)
}

func getBillIds(bills []*Bill) [][]byte {
	var billIds [][]byte
	for _, b := range bills {
		billIds = append(billIds, b.GetID())
	}
	return billIds
}

func convertBills(billsList []*backendmoney.ListBillVM) ([]*Bill, error) {
	var bills []*Bill
	for _, b := range billsList {
		bill := newBillFromVM(b)
		bills = append(bills, bill)
	}
	return bills, nil
}

// newBillFromVM converts ListBillVM to Bill structs
func newBillFromVM(b *backendmoney.ListBillVM) *Bill {
	return &Bill{
		Id:       util.BytesToUint256(b.Id),
		Value:    b.Value,
		TxHash:   b.TxHash,
		IsDcBill: b.IsDCBill,
		DcNonce:  hashId(b.Id),
	}
}

// newBill creates new Bill struct from given BlockProof for Transfer and Split transactions.
func newBill(proof *BlockProof, txConverter *TxConverter) (*Bill, error) {
	gtx, err := txConverter.ConvertTx(proof.Tx)
	if err != nil {
		return nil, err
	}
	switch tx := gtx.(type) {
	case money.Transfer:
		return &Bill{
			Id:         tx.UnitID(),
			Value:      tx.TargetValue(),
			TxHash:     tx.Hash(crypto.SHA256),
			BlockProof: proof,
		}, nil
	case money.Split:
		return &Bill{
			Id:         txutil.SameShardID(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256)),
			Value:      tx.Amount(),
			TxHash:     tx.Hash(crypto.SHA256),
			BlockProof: proof,
		}, nil
	default:
		return nil, errors.New("cannot convert unsupported tx type to Bill struct")
	}
}

func convertBill(b *bp.Bill) *Bill {
	if b.IsDcBill {
		attrs := &money.TransferDCAttributes{}
		err := b.TxProof.Tx.TransactionAttributes.UnmarshalTo(attrs)
		if err != nil {
			return nil
		}
		return &Bill{Id: util.BytesToUint256(b.Id), Value: b.Value, TxHash: b.TxHash, IsDcBill: b.IsDcBill, DcNonce: attrs.Nonce, DcTimeout: b.TxProof.Tx.Timeout(), BlockProof: &BlockProof{Tx: b.TxProof.Tx, Proof: b.TxProof.Proof, BlockNumber: b.TxProof.BlockNumber}}
	}
	return &Bill{Id: util.BytesToUint256(b.Id), Value: b.Value, TxHash: b.TxHash, IsDcBill: b.IsDcBill, BlockProof: &BlockProof{Tx: b.TxProof.Tx, Proof: b.TxProof.Proof, BlockNumber: b.TxProof.BlockNumber}}
}
