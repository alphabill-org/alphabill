package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/internal/block"
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
	dcTimeoutBlockCount     = 10
	swapTimeoutBlockCount   = 60
	txTimeoutBlockCount     = 100
	dustBillDeletionTimeout = 65536
)

var (
	ErrSwapInProgress       = errors.New("swap is in progress, synchronize your wallet to complete the process")
	ErrInsufficientBalance  = errors.New("insufficient balance for transaction")
	ErrInvalidPubKey        = errors.New("invalid public key, public key must be in compressed secp256k1 format")
	ErrInvalidAmount        = errors.New("invalid amount")
	ErrInvalidAccountIndex  = errors.New("invalid account index")
	ErrInvalidBlockSystemID = errors.New("invalid system identifier")
	ErrTxFailedToConfirm    = errors.New("transaction(s) failed to confirm")
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

	maxBlockNo, err := w.GetMaxBlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	timeout := maxBlockNo + txTimeoutBlockCount
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

	apiWrapper := &backendAPIWrapper{wallet: w}
	batch := txsubmitter.NewBatch(k.PubKey, apiWrapper)

	if err = createTransactions(batch.Add, txConverter, cmd.ReceiverPubKey, cmd.Amount, bills, k, timeout); err != nil {
		return nil, err
	}
	if err = batch.SendTx(ctx, cmd.WaitForConfirmation); err != nil {
		return nil, err
	}
	return apiWrapper.txProofs, nil
}

type backendAPIWrapper struct {
	wallet   *Wallet
	txProofs []*Bill
}

func (b *backendAPIWrapper) GetRoundNumber(context.Context) (uint64, error) {
	return b.wallet.restClient.GetBlockHeight()
}

func (b *backendAPIWrapper) PostTransactions(ctx context.Context, _ wallet.PubKey, txs *txsystem.Transactions) error {
	for _, tx := range txs.Transactions {
		err := b.wallet.SendTransaction(ctx, tx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *backendAPIWrapper) GetTxProof(_ context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	resp, err := b.wallet.restClient.GetProof(unitID)
	if err != nil {
		return nil, err
	}
	if len(resp.Bills) != 1 {
		return nil, errors.New(fmt.Sprintf("unexpected number of proofs: %d, bill ID: %X", len(resp.Bills), unitID))
	}
	bill := resp.Bills[0]
	if !bytes.Equal(bill.TxHash, txHash) {
		// confirmation expects nil (not error) if there's no proof for the given tx hash (yet)
		return nil, nil
	}
	b.txProofs = append(b.txProofs, convertBill(bill))
	proof := bill.GetTxProof()
	return &wallet.Proof{
		BlockNumber: proof.BlockNumber,
		Tx:          proof.Tx,
		Proof:       proof.Proof,
	}, nil
}

// collectDust sends dust transfer for every bill for given account in wallet and records metadata.
// Returns immediately without error if there's already 1 or 0 bills.
// Once the dust transfers get confirmed on the ledger then swap transfer is broadcast and metadata cleared.
// If blocking is true then the function blocks until swap has been completed or timed out,
// if blocking is false then the function returns after sending the dc transfers.
func (w *Wallet) collectDust(ctx context.Context, blocking bool, accountIndex uint64) error {
	log.Info("starting dust collection for account=", accountIndex, " blocking=", blocking)
	blockHeight, err := w.GetMaxBlockNumber(ctx)
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
	var bills []*Bill
	for _, b := range billResponse.Bills {
		proof, err := w.restClient.GetProof(b.Id)
		if err != nil {
			return err
		}
		bills = append(bills, convertBill(proof.Bills[0]))
	}
	var expectedSwaps []expectedSwap
	dcBills := collectDcBills(bills)
	dcNonce := calculateDcNonce(bills)
	if len(dcBills) > 0 {
		swapTimeout := blockHeight + swapTimeoutBlockCount
		err = w.swapDcBills(dcBills, dcNonce, getBillIds(dcBills), swapTimeout, accountIndex)
		if err != nil {
			return err
		}
		expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: dcNonce, timeout: swapTimeout})
	} else {
		k, err := w.am.GetAccountKey(accountIndex)
		if err != nil {
			return err
		}

		dcTimeout := blockHeight + dcTimeoutBlockCount
		for _, b := range bills {
			tx, err := createDustTx(k, b, dcNonce, dcTimeout)
			if err != nil {
				return err
			}
			log.Info("sending dust transfer tx for bill=", b.Id, " account=", accountIndex)
			err = w.SendTransaction(ctx, tx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
			if err != nil {
				return err
			}
		}
		expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: dcNonce, timeout: dcTimeout})
	}
	if blocking {
		w.dcWg.AddExpectedSwaps(expectedSwaps)
	}
	if blocking {
		log.Info("waiting for blocking collect dust on account=", accountIndex, " (wallet needs to be synchronizing to finish this process)")

		// wrap wg.Wait() as channel
		done := make(chan struct{})
		go func() {
			w.dcWg.wg.Wait()
			close(done)
		}()

		select {
		case <-ctx.Done():
			// context canceled externally
		case <-done:
			// dust collection finished (swap received or timed out)
		}
		log.Info("finished waiting for blocking collect dust on account=", accountIndex)
	}
	return nil
}

func (w *Wallet) swapDcBills(dcBills []*Bill, dcNonce []byte, billIds [][]byte, timeout uint64, accountIndex uint64) error {
	k, err := w.am.GetAccountKey(accountIndex)
	if err != nil {
		return err
	}
	swap, err := createSwapTx(k, dcBills, dcNonce, billIds, timeout)
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

func (w *Wallet) waitForConfirmation(ctx context.Context, pendingTxs []*txsystem.Transaction, maxBlockNumber, timeout uint64) ([]*Bill, error) {
	log.Info("waiting for confirmation(s)...")
	blockNumber := maxBlockNumber
	txsLog := NewTxLog(pendingTxs)
	for blockNumber <= timeout {
		b, err := w.AlphabillClient.GetBlock(ctx, blockNumber)
		if err != nil {
			return nil, err
		}
		if b == nil {
			// wait for some time before retrying to fetch new block
			timer := time.NewTimer(100 * time.Millisecond)
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
				err = txsLog.RecordTx(tx, b)
				if err != nil {
					return nil, err
				}
				if txsLog.IsAllTxsConfirmed() {
					log.Info("transaction(s) confirmed")
					return txsLog.GetAllRecordedBills(), nil
				}
			}
		}
		blockNumber += 1
	}
	return nil, ErrTxFailedToConfirm
}

func (s *SendCmd) isValid() error {
	if len(s.ReceiverPubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
		return ErrInvalidPubKey
	}
	if s.Amount < 0 {
		return ErrInvalidAmount
	}
	if s.AccountIndex < 0 {
		return ErrInvalidAccountIndex
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

func collectDcBills(bills []*Bill) []*Bill {
	var dcBills []*Bill
	for _, b := range bills {
		if b.IsDcBill {
			dcBills = append(dcBills, b)
		}
	}
	return dcBills
}

func convertBills(billsList []*backendmoney.ListBillVM) ([]*Bill, error) {
	var bills []*Bill
	for _, b := range billsList {
		billValueUint, err := strconv.ParseUint(b.Value, 10, 64)
		if err != nil {
			return nil, err
		}
		bill := &Bill{Id: util.BytesToUint256(b.Id), Value: billValueUint, TxHash: b.TxHash, IsDcBill: b.IsDCBill, DcNonce: hashId(b.Id)}
		bills = append(bills, bill)
	}
	return bills, nil
}

func convertBill(b *block.Bill) *Bill {
	return &Bill{Id: util.BytesToUint256(b.Id), Value: b.Value, TxHash: b.TxHash, IsDcBill: b.IsDcBill, DcNonce: hashId(b.Id), BlockProof: &BlockProof{Tx: b.TxProof.Tx, Proof: b.TxProof.Proof, BlockNumber: b.TxProof.BlockNumber}}
}
