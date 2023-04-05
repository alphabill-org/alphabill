package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"
	"time"

	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"golang.org/x/sync/errgroup"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	abclient "github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	backendmoney "github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

const (
	dcTimeoutBlockCount       = 10
	swapTimeoutBlockCount     = 60
	txTimeoutBlockCount       = 100
	maxBillsForDustCollection = 100
	dustBillDeletionTimeout   = 65536
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance for transaction")
	ErrInvalidPubKey       = errors.New("invalid public key, public key must be in compressed secp256k1 format")
	ErrInvalidPassword     = errors.New("invalid password")
	ErrTxFailedToConfirm   = errors.New("transaction(s) failed to confirm")
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
func (w *Wallet) CollectDust(ctx context.Context) error {
	errgrp, ctx := errgroup.WithContext(ctx)
	for _, acc := range w.am.GetAll() {
		acc := acc // copy value for closure
		errgrp.Go(func() error {
			return w.collectDust(ctx, true, acc.AccountIndex)
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

	_, roundNumber, err := w.GetMaxBlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	timeout := roundNumber + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	k, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	billResponse, err := w.restClient.ListBills(pubKey)
	if err != nil {
		return nil, err
	}
	bills, err := convertBills(billResponse.Bills)
	if err != nil {
		return nil, err
	}

	txs, err := createTransactions(cmd.ReceiverPubKey, cmd.Amount, w.SystemID(), bills, k, timeout)
	if err != nil {
		return nil, err
	}
	for _, tx := range txs {
		err := w.SendTransaction(ctx, tx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
		if err != nil {
			return nil, err
		}
	}
	if cmd.WaitForConfirmation {
		txProofs, err := w.waitForConfirmation(ctx, txs, roundNumber, timeout)
		if err != nil {
			return nil, err
		}
		return txProofs, nil
	}
	return nil, nil
}

// collectDust sends dust transfer for every bill for given account in wallet and records metadata.
// Returns immediately without error if there's already 1 or 0 bills.
// Once the dust transfers get confirmed on the ledger then swap transfer is broadcast and metadata cleared.
// If blocking is true then the function blocks until swap has been completed or timed out,
// if blocking is false then the function returns after sending the dc transfers.
func (w *Wallet) collectDust(ctx context.Context, blocking bool, accountIndex uint64) error {
	log.Info("starting dust collection for account=", accountIndex, " blocking=", blocking)
	_, roundNr, err := w.GetMaxBlockNumber(ctx)
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
	_, roundNr, err := w.GetMaxBlockNumber(ctx)
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
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-timer.C:
			_, roundNr, err = w.GetMaxBlockNumber(ctx)
			if err != nil {
				return err
			}
			continue
		case <-ctx.Done():
			timer.Stop()
			return nil
		}
	}
	return nil
}

func (w *Wallet) confirmSwap(ctx context.Context) error {
	_, roundNr, err := w.GetMaxBlockNumber(ctx)
	if err != nil {
		return err
	}
	log.Info("waiting for swap confirmation(s)...")
	swapTimeout := roundNr + swapTimeoutBlockCount
	for roundNr <= swapTimeout {
		println(roundNr)
		if len(w.dcWg.swaps) == 0 {
			return nil
		}
		b, err := w.AlphabillClient.GetBlock(ctx, roundNr)
		if err != nil {
			return err
		}
		if b != nil {
			for _, tx := range b.Transactions {
				err = w.dcWg.DecrementSwaps(string(tx.UnitId), roundNr)
				if err != nil {
					return err
				}
			}
			if len(w.dcWg.swaps) == 0 {
				return nil
			}
		}
		// wait for some time before retrying to fetch new block
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-timer.C:
			_, roundNr, err = w.GetMaxBlockNumber(ctx)
			if err != nil {
				return err
			}
			continue
		case <-ctx.Done():
			timer.Stop()
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

func (w *Wallet) waitForConfirmation(ctx context.Context, pendingTxs []*txsystem.Transaction, latestRoundNumber, timeout uint64) ([]*Bill, error) {
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
			_, latestRoundNumber, err = w.AlphabillClient.GetMaxBlockNumber(ctx)
			if err != nil {
				return nil, err
			}
			if latestRoundNumber > latestBlockNumber {
				latestBlockNumber++
				continue
			}
			// wait for some time before retrying to fetch new block
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-timer.C:
				continue
			case <-ctx.Done():
				timer.Stop()
				return nil, nil
			}
		}
		for _, tx := range b.Transactions {
			if txsLog.Contains(tx) {
				log.Info("confirmed tx ", hexutil.Encode(tx.UnitId))
				err = txsLog.RecordTx(tx, b, NewTxConverter(w.SystemID()))
				if err != nil {
					return nil, err
				}
				if txsLog.IsAllTxsConfirmed() {
					log.Info("transaction(s) confirmed")
					return txsLog.GetAllRecordedBills(), nil
				}
			}
		}
		latestBlockNumber++
	}
	return nil, ErrTxFailedToConfirm
}

func (s *SendCmd) isValid() error {
	if len(s.ReceiverPubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
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

func calculateDcNonce(bills []*Bill) []byte {
	var billIds [][]byte
	for _, b := range bills {
		billIds = append(billIds, b.GetID())
	}

	// sort billIds in ascending order
	sort.Slice(billIds, func(i, j int) bool {
		return bytes.Compare(billIds[i], billIds[j]) < 0
	})

	hasher := crypto.Hash.New(crypto.SHA256)
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
		bill := &Bill{Id: util.BytesToUint256(b.Id), Value: b.Value, TxHash: b.TxHash, IsDcBill: b.IsDCBill, DcNonce: hashId(b.Id)}
		bills = append(bills, bill)
	}
	return bills, nil
}

func convertBill(b *moneytx.Bill) *Bill {
	if b.IsDcBill {
		attrs := &moneytx.TransferDCOrder{}
		err := b.TxProof.Tx.TransactionAttributes.UnmarshalTo(attrs)
		if err != nil {
			return nil
		}
		return &Bill{Id: util.BytesToUint256(b.Id), Value: b.Value, TxHash: b.TxHash, IsDcBill: b.IsDcBill, DcNonce: attrs.Nonce, DcTimeout: b.TxProof.Tx.Timeout, BlockProof: &BlockProof{Tx: b.TxProof.Tx, Proof: b.TxProof.Proof, BlockNumber: b.TxProof.BlockNumber}}
	}
	return &Bill{Id: util.BytesToUint256(b.Id), Value: b.Value, TxHash: b.TxHash, IsDcBill: b.IsDcBill, BlockProof: &BlockProof{Tx: b.TxProof.Tx, Proof: b.TxProof.Proof, BlockNumber: b.TxProof.BlockNumber}}
}
