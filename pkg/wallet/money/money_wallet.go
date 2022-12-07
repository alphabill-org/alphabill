package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	txverifier "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_verifier"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

const (
	dcTimeoutBlockCount     = 10
	swapTimeoutBlockCount   = 60
	txTimeoutBlockCount     = 100
	dustBillDeletionTimeout = 300
)

var (
	ErrSwapInProgress       = errors.New("swap is in progress, synchronize your wallet to complete the process")
	ErrInsufficientBalance  = errors.New("insufficient balance for transaction")
	ErrInvalidPubKey        = errors.New("invalid public key, public key must be in compressed secp256k1 format")
	ErrInvalidAmount        = errors.New("invalid amount")
	ErrInvalidAccountIndex  = errors.New("invalid account index")
	ErrInvalidPassword      = errors.New("invalid password")
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

		config           WalletConfig
		db               Db
		dustCollectorJob *cron.Cron
		dcWg             *dcWaitGroup
		accounts         *accounts
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
func CreateNewWallet(mnemonic string, config WalletConfig) (*Wallet, error) {
	db, err := getDb(config, true)
	if err != nil {
		return nil, err
	}
	return createMoneyWallet(config, db, mnemonic)
}

func LoadExistingWallet(config WalletConfig) (*Wallet, error) {
	db, err := getDb(config, false)
	if err != nil {
		return nil, err
	}

	ok, err := db.Do().VerifyPassword()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidPassword
	}

	accountKeys, err := db.Do().GetAccountKeys()
	if err != nil {
		return nil, err
	}
	accs := make([]account, len(accountKeys))
	for idx, val := range accountKeys {
		accs[idx] = account{
			accountIndex: uint64(idx),
			accountKeys:  *val.PubKeyHash,
		}
	}
	mw := &Wallet{config: config, db: db, dustCollectorJob: cron.New(), dcWg: newDcWaitGroup(), accounts: &accounts{accounts: accs}}

	tb, err := config.GetTrustBase()
	if err != nil {
		return nil, err
	}
	mw.Wallet, err = wallet.New(
		wallet.WithBlockProcessor(mw),
		wallet.WithAlphabillClientConfig(config.AlphabillClientConfig),
		wallet.WithTrustBase(tb),
	)
	if err != nil {
		return nil, err
	}
	return mw, nil
}

// IsEncrypted returns true if wallet exists and is encrypted and or false if wallet exists and is not encrypted,
// returns error if wallet does not exist.
func IsEncrypted(config WalletConfig) (bool, error) {
	db, err := getDb(config, false)
	if err != nil {
		return false, err
	}
	defer db.Close()
	return db.Do().IsEncrypted()
}

func (w *Wallet) ProcessBlock(b *block.Block) error {
	log.Info("processing block: ", b.BlockNumber)
	if !bytes.Equal(alphabillMoneySystemId, b.GetSystemIdentifier()) {
		return ErrInvalidBlockSystemID
	}

	return w.db.WithTransaction(func(dbTx TxContext) error {
		lastBlockNumber, err := w.db.Do().GetBlockNumber()
		if err != nil {
			return err
		}
		err = validateBlockNumber(b.BlockNumber, lastBlockNumber)
		if err != nil {
			return err
		}
		for _, acc := range w.accounts.getAll() {
			for _, pbTx := range b.Transactions {
				err = w.collectBills(dbTx, pbTx, b, &acc)
				if err != nil {
					return err
				}
			}
		}
		return w.endBlock(dbTx, b)
	})
}

func (w *Wallet) endBlock(dbTx TxContext, b *block.Block) error {
	blockNumber := b.BlockNumber
	err := dbTx.SetBlockNumber(blockNumber)
	if err != nil {
		return err
	}
	for _, acc := range w.accounts.getAll() {
		err = w.deleteExpiredDcBills(dbTx, blockNumber, acc.accountIndex)
		if err != nil {
			return err
		}
		err = w.trySwap(dbTx, acc.accountIndex)
		if err != nil {
			return err
		}
		err = w.dcWg.DecrementSwaps(dbTx, blockNumber, acc.accountIndex)
		if err != nil {
			return err
		}
	}
	return nil
}

// Shutdown terminates connection to alphabill node, closes wallet db, cancels dust collector job and any background goroutines.
func (w *Wallet) Shutdown() {
	w.Wallet.Shutdown()

	if w.dustCollectorJob != nil {
		w.dustCollectorJob.Stop()
	}
	if w.dcWg != nil {
		w.dcWg.ResetWaitGroup()
	}
	if w.db != nil {
		w.db.Close()
	}
}

// DeleteDb deletes the wallet database.
func (w *Wallet) DeleteDb() {
	if w.db != nil {
		w.db.DeleteDb()
	}
}

// CollectDust starts the dust collector process for all accounts in the wallet.
// Wallet needs to be synchronizing using Sync or SyncToMaxBlockNumber in order to receive transactions and finish the process.
// The function blocks until dust collector process is finished or timed out. Skips account if the account already has only one or no bills.
func (w *Wallet) CollectDust(ctx context.Context) error {
	errgrp, ctx := errgroup.WithContext(ctx)
	for _, acc := range w.accounts.getAll() {
		acc := acc // copy value for closure
		errgrp.Go(func() error {
			return w.collectDust(ctx, true, acc.accountIndex)
		})
	}
	return errgrp.Wait()
}

// StartDustCollectorJob starts the dust collector background process that runs every hour until wallet is shut down.
// Wallet needs to be synchronizing using Sync or SyncToMaxBlockNumber in order to receive transactions and finish the process.
// Returns error if the job failed to start.
func (w *Wallet) StartDustCollectorJob() error {
	_, err := w.startDustCollectorJob()
	return err
}

// GetBalance returns sum value of all bills currently owned by the wallet, for given account.
// The value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalance(cmd GetBalanceCmd) (uint64, error) {
	return w.db.Do().GetBalance(cmd)
}

// GetBalances returns sum value of all bills currently owned by the wallet, for all accounts.
// The value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalances(cmd GetBalanceCmd) ([]uint64, error) {
	return w.db.Do().GetBalances(cmd)
}

// GetBill returns bill for the given bill id.
// If bill does not exist returns error "bill does not exist".
func (w *Wallet) GetBill(accountIndex uint64, billId []byte) (*Bill, error) {
	return w.db.Do().GetBill(accountIndex, billId)
}

// GetBills returns all bills owned by the wallet for the given account.
func (w *Wallet) GetBills(accountIndex uint64) ([]*Bill, error) {
	return w.db.Do().GetBills(accountIndex)
}

// GetAllBills returns all bills owned by the wallet for all accounts.
func (w *Wallet) GetAllBills() ([][]*Bill, error) {
	return w.db.Do().GetAllBills()
}

// AddBill adds bill to wallet.
// Given bill must have a valid transaction with P2PKH predicate for given account.
// Block proof is not verified, but transaction is required.
// Overwrites existing bill with the same ID, if one exists.
func (w *Wallet) AddBill(accountIndex uint64, bill *Bill) error {
	if bill == nil {
		return errors.New("bill is nil")
	}
	if bill.Id == nil {
		return errors.New("bill id is nil")
	}
	if bill.TxHash == nil {
		return errors.New("bill tx hash is nil")
	}
	if bill.BlockProof == nil {
		return errors.New("bill block proof is nil")
	}
	tx := bill.BlockProof.Tx
	if tx == nil {
		return errors.New("bill block proof tx is nil")
	}
	key, err := w.db.Do().GetAccountKey(accountIndex)
	if err != nil {
		return err
	}
	gtx, err := txConverter.ConvertTx(tx)
	if err != nil {
		return err
	}
	err = txverifier.VerifyTxP2PKHOwner(gtx, key.PubKeyHash)
	if err != nil {
		return err
	}
	return w.db.Do().SetBill(accountIndex, bill)
}

// GetPublicKey returns public key of the wallet (compressed secp256k1 key 33 bytes)
func (w *Wallet) GetPublicKey(accountIndex uint64) ([]byte, error) {
	key, err := w.GetAccountKey(accountIndex)
	if err != nil {
		return nil, err
	}
	return key.PubKey, nil
}

func (w *Wallet) GetAccountKey(accountIndex uint64) (*wallet.AccountKey, error) {
	key, err := w.db.Do().GetAccountKey(accountIndex)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (w *Wallet) GetAccountKeys() ([]*wallet.AccountKey, error) {
	accKeys, err := w.db.Do().GetAccountKeys()
	if err != nil {
		return nil, err
	}
	return accKeys, nil
}

// GetPublicKeys returns public keys of the wallet, indexed by account indexes
func (w *Wallet) GetPublicKeys() ([][]byte, error) {
	accKeys, err := w.GetAccountKeys()
	if err != nil {
		return nil, err
	}
	pubKeys := make([][]byte, len(accKeys))
	for accIdx, accKey := range accKeys {
		pubKeys[accIdx] = accKey.PubKey
	}
	return pubKeys, nil
}

// GetMnemonic returns mnemonic seed of the wallet
func (w *Wallet) GetMnemonic() (string, error) {
	return w.db.Do().GetMnemonic()
}

// AddAccount adds the next account in account key series to the wallet.
// New accounts are indexed only from the time of creation and not backwards in time.
// Returns new account's index and public key.
func (w *Wallet) AddAccount() (uint64, []byte, error) {
	masterKeyString, err := w.db.Do().GetMasterKey()
	if err != nil {
		return 0, nil, err
	}
	masterKey, err := hdkeychain.NewKeyFromString(masterKeyString)
	if err != nil {
		return 0, nil, err
	}

	accountIndex, err := w.db.Do().GetMaxAccountIndex()
	if err != nil {
		return 0, nil, err
	}
	accountIndex += 1

	derivationPath := wallet.NewDerivationPath(accountIndex)
	accountKey, err := wallet.NewAccountKey(masterKey, derivationPath)
	if err != nil {
		return 0, nil, err
	}
	err = w.db.WithTransaction(func(tx TxContext) error {
		err := tx.AddAccount(accountIndex, accountKey)
		if err != nil {
			return err
		}
		err = tx.SetMaxAccountIndex(accountIndex)
		if err != nil {
			return err
		}
		w.accounts.add(&account{accountIndex: accountIndex, accountKeys: *accountKey.PubKeyHash})
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	return accountIndex, accountKey.PubKey, nil
}

// Send creates, signs and broadcasts transactions, in total for the given amount,
// to the given public key, the public key must be in compressed secp256k1 format.
// Sends one transaction per bill, prioritzing larger bills.
// Returns list of bills including transaction and proof data, if waitForConfirmation=true, otherwise nil.
func (w *Wallet) Send(ctx context.Context, cmd SendCmd) ([]*Bill, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}

	swapInProgress, err := w.isSwapInProgress(w.db.Do(), cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	if swapInProgress {
		return nil, ErrSwapInProgress
	}

	balance, err := w.GetBalance(GetBalanceCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
		return nil, err
	}
	if cmd.Amount > balance {
		return nil, ErrInsufficientBalance
	}

	maxBlockNo, err := w.GetMaxBlockNumber()
	if err != nil {
		return nil, err
	}
	timeout := maxBlockNo + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	k, err := w.db.Do().GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	bills, err := w.db.Do().GetBills(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	txs, err := createTransactions(cmd.ReceiverPubKey, cmd.Amount, bills, k, timeout)
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
		txProofs, err := w.waitForConfirmation(ctx, txs, maxBlockNo, timeout, cmd.AccountIndex)
		if err != nil {
			return nil, err
		}
		return txProofs, nil
	}
	return nil, nil
}

// Sync synchronises wallet from the last known block number with the given alphabill node.
// The function blocks forever or until alphabill connection is terminated.
// Returns immediately if already synchronizing.
func (w *Wallet) Sync(ctx context.Context) error {
	blockNumber, err := w.db.Do().GetBlockNumber()
	if err != nil {
		return err
	}
	return w.Wallet.Sync(ctx, blockNumber)
}

// SyncToMaxBlockNumber synchronises wallet from the last known block number with the given alphabill node.
// The function blocks until maximum block height, calculated at the start of the process, is reached.
// Returns immediately with ErrWalletAlreadySynchronizing if already synchronizing.
func (w *Wallet) SyncToMaxBlockNumber(ctx context.Context) error {
	blockNumber, err := w.db.Do().GetBlockNumber()
	if err != nil {
		return err
	}
	return w.Wallet.SyncToMaxBlockNumber(ctx, blockNumber)
}

func (w *Wallet) collectBills(dbTx TxContext, txPb *txsystem.Transaction, b *block.Block, acc *account) error {
	gtx, err := moneytx.NewMoneyTx(alphabillMoneySystemId, txPb)
	if err != nil {
		return err
	}
	stx := gtx.(txsystem.GenericTransaction)
	genericBlock, err := b.ToGenericBlock(&TxConverter{})
	if err != nil {
		return fmt.Errorf("invalid block: %v", err)
	}
	err = w.TxVerifier.Verify(gtx, genericBlock)
	if err != nil {
		return fmt.Errorf("invalid tx: %v", err)
	}

	switch tx := stx.(type) {
	case money.Transfer:
		if wallet.VerifyP2PKHOwner(&acc.accountKeys, tx.NewBearer()) {
			log.Info("received transfer order")
			err := w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.accountIndex)
			if err != nil {
				return err
			}
		} else {
			err := dbTx.RemoveBill(acc.accountIndex, tx.UnitID())
			if err != nil {
				return err
			}
		}
	case money.TransferDC:
		if wallet.VerifyP2PKHOwner(&acc.accountKeys, tx.TargetBearer()) {
			log.Info("received TransferDC order")
			err := w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:                  tx.UnitID(),
				Value:               tx.TargetValue(),
				TxHash:              tx.Hash(crypto.SHA256),
				IsDcBill:            true,
				DcTimeout:           tx.Timeout(),
				DcNonce:             tx.Nonce(),
				DcExpirationTimeout: b.BlockNumber + dustBillDeletionTimeout,
			}, acc.accountIndex)
			if err != nil {
				return err
			}
		} else {
			err := dbTx.RemoveBill(acc.accountIndex, tx.UnitID())
			if err != nil {
				return err
			}
		}
	case money.Split:
		// split tx contains two bills: existing bill and new bill
		// if any of these bills belong to wallet then we have to
		// 1) update the existing bill and
		// 2) add the new bill
		containsBill, err := dbTx.ContainsBill(acc.accountIndex, tx.UnitID())
		if err != nil {
			return err
		}
		if containsBill {
			log.Info("received split order (existing bill)")
			err := w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:     tx.UnitID(),
				Value:  tx.RemainingValue(),
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.accountIndex)
			if err != nil {
				return err
			}
		}
		if wallet.VerifyP2PKHOwner(&acc.accountKeys, tx.TargetBearer()) {
			log.Info("received split order (new bill)")
			err := w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:     util.SameShardID(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256)),
				Value:  tx.Amount(),
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.accountIndex)
			if err != nil {
				return err
			}
		}
	case money.Swap:
		if wallet.VerifyP2PKHOwner(&acc.accountKeys, tx.OwnerCondition()) {
			log.Info("received swap order")
			err := w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.accountIndex)
			if err != nil {
				return err
			}
			// clear dc metadata
			err = dbTx.SetDcMetadata(acc.accountIndex, txPb.UnitId, nil)
			if err != nil {
				return err
			}
			for _, dustTransfer := range tx.DCTransfers() {
				err := dbTx.RemoveBill(acc.accountIndex, dustTransfer.UnitID())
				if err != nil {
					return err
				}
			}
		} else {
			err := dbTx.RemoveBill(acc.accountIndex, tx.UnitID())
			if err != nil {
				return err
			}
		}
	default:
		log.Warning(fmt.Sprintf("received unknown transaction type, skipping processing: %s", tx))
		return nil
	}
	return nil
}

func (w *Wallet) saveWithProof(dbTx TxContext, b *block.Block, txPb *txsystem.Transaction, bill *Bill, accountIndex uint64) error {
	err := bill.addProof(b, txPb)
	if err != nil {
		return err
	}
	return dbTx.SetBill(accountIndex, bill)
}

func (w *Wallet) deleteExpiredDcBills(dbTx TxContext, blockNumber uint64, accountIndex uint64) error {
	bills, err := dbTx.GetBills(accountIndex)
	if err != nil {
		return err
	}
	for _, b := range bills {
		if b.isExpired(blockNumber) {
			log.Info(fmt.Sprintf("deleting expired dc bill: value=%d id=%s", b.Value, b.Id.String()))
			err = dbTx.RemoveBill(accountIndex, b.Id)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Wallet) trySwap(tx TxContext, accountIndex uint64) error {
	blockHeight, err := tx.GetBlockNumber()
	if err != nil {
		return err
	}
	bills, err := tx.GetBills(accountIndex)
	if err != nil {
		return err
	}
	dcBillGroups := groupDcBills(bills)
	for nonce, billGroup := range dcBillGroups {
		nonce32 := nonce.Bytes32()
		dcMeta, err := tx.GetDcMetadata(accountIndex, nonce32[:])
		if err != nil {
			return err
		}
		if dcMeta != nil && dcMeta.isSwapRequired(blockHeight, billGroup.valueSum) {
			maxBlockNumber, err := w.GetMaxBlockNumber()
			if err != nil {
				return err
			}
			timeout := maxBlockNumber + swapTimeoutBlockCount
			err = w.swapDcBills(tx, billGroup.dcBills, billGroup.dcNonce, dcMeta.BillIds, timeout, accountIndex)
			if err != nil {
				return err
			}
			w.dcWg.UpdateTimeout(billGroup.dcNonce, timeout)
		}
	}

	// delete expired metadata
	nonceMetadataMap, err := tx.GetDcMetadataMap(accountIndex)
	if err != nil {
		return err
	}
	for nonce, m := range nonceMetadataMap {
		if m.timeoutReached(blockHeight) {
			nonce32 := nonce.Bytes32()
			err := tx.SetDcMetadata(accountIndex, nonce32[:], nil)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// collectDust sends dust transfer for every bill for given account in wallet and records metadata.
// Returns immediately without error if there's already 1 or 0 bills.
// Once the dust transfers get confirmed on the ledger then swap transfer is broadcast and metadata cleared.
// If blocking is true then the function blocks until swap has been completed or timed out,
// if blocking is false then the function returns after sending the dc transfers.
func (w *Wallet) collectDust(ctx context.Context, blocking bool, accountIndex uint64) error {
	err := w.db.WithTransaction(func(dbTx TxContext) error {
		log.Info("starting dust collection for account=", accountIndex, " blocking=", blocking)
		blockHeight, err := dbTx.GetBlockNumber()
		if err != nil {
			return err
		}
		maxBlockNo, err := w.GetMaxBlockNumber()
		if err != nil {
			return err
		}
		bills, err := dbTx.GetBills(accountIndex)
		if err != nil {
			return err
		}
		if len(bills) < 2 {
			log.Info("Account ", accountIndex, " has less than 2 bills, skipping dust collection")
			return nil
		}
		var expectedSwaps []expectedSwap
		dcBillGroups := groupDcBills(bills)
		if len(dcBillGroups) > 0 {
			for _, v := range dcBillGroups {
				if blockHeight >= v.dcTimeout {
					swapTimeout := maxBlockNo + swapTimeoutBlockCount
					billIds, err := getBillIds(dbTx, accountIndex, v)
					if err != nil {
						return err
					}
					err = w.swapDcBills(dbTx, v.dcBills, v.dcNonce, billIds, swapTimeout, accountIndex)
					if err != nil {
						return err
					}
					expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: v.dcNonce, timeout: swapTimeout})
				} else {
					// expecting to receive swap during dcTimeout
					expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: v.dcNonce, timeout: v.dcTimeout})
				}
			}
		} else {
			swapInProgress, err := w.isSwapInProgress(dbTx, accountIndex)
			if err != nil {
				return err
			}
			if swapInProgress {
				return ErrSwapInProgress
			}

			k, err := dbTx.GetAccountKey(accountIndex)
			if err != nil {
				return err
			}

			dcNonce := calculateDcNonce(bills)
			dcTimeout := maxBlockNo + dcTimeoutBlockCount
			var dcValueSum uint64
			var billIds [][]byte
			for _, b := range bills {
				dcValueSum += b.Value
				billIds = append(billIds, b.GetID())
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
			err = dbTx.SetDcMetadata(accountIndex, dcNonce, &dcMetadata{
				DcValueSum: dcValueSum,
				BillIds:    billIds,
				DcTimeout:  dcTimeout,
			})
			if err != nil {
				return err
			}
		}
		if blocking {
			w.dcWg.AddExpectedSwaps(expectedSwaps)
		}
		return nil
	})
	if err != nil {
		return err
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

func (w *Wallet) swapDcBills(tx TxContext, dcBills []*Bill, dcNonce []byte, billIds [][]byte, timeout uint64, accountIndex uint64) error {
	k, err := tx.GetAccountKey(accountIndex)
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
	return tx.SetDcMetadata(accountIndex, dcNonce, &dcMetadata{SwapTimeout: timeout, BillIds: billIds})
}

// isSwapInProgress returns true if there's a running dc process managed by the wallet, for the given account
func (w *Wallet) isSwapInProgress(dbTx TxContext, accIdx uint64) (bool, error) {
	blockHeight, err := dbTx.GetBlockNumber()
	if err != nil {
		return false, err
	}
	dcMetadataMap, err := dbTx.GetDcMetadataMap(accIdx)
	if err != nil {
		return false, err
	}
	for _, m := range dcMetadataMap {
		if m.DcValueSum > 0 { // value sum is set only for dc process that was started by wallet
			return blockHeight < m.DcTimeout || blockHeight < m.SwapTimeout, nil
		}
	}
	return false, nil
}

func (w *Wallet) startDustCollectorJob() (cron.EntryID, error) {
	return w.dustCollectorJob.AddFunc("@hourly", func() {
		for _, acc := range w.accounts.getAll() {
			err := w.collectDust(context.Background(), false, acc.accountIndex)
			if err != nil {
				log.Error("error in dust collector job: ", err)
			}
		}
	})
}

func (w *Wallet) waitForConfirmation(ctx context.Context, pendingTxs []*txsystem.Transaction, maxBlockNumber, timeout, accountIndex uint64) ([]*Bill, error) {
	log.Info("waiting for confirmation(s)...")
	blockNumber := maxBlockNumber
	txsLog := newTxLog(pendingTxs)
	for blockNumber <= timeout {
		b, err := w.AlphabillClient.GetBlock(blockNumber)
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
			if txsLog.contains(tx) {
				log.Info("confirmed tx ", hexutil.Encode(tx.UnitId))
				err = w.collectBills(w.db.Do(), tx, b, &w.accounts.getAll()[accountIndex])
				if err != nil {
					return nil, err
				}
				err = txsLog.recordTx(tx, b)
				if err != nil {
					return nil, err
				}
				if txsLog.isAllTxsConfirmed() {
					log.Info("transaction(s) confirmed")
					return txsLog.getAllRecordedBills(), nil
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

func createMoneyWallet(config WalletConfig, db Db, mnemonic string) (mw *Wallet, err error) {
	mw = &Wallet{config: config, db: db, dustCollectorJob: cron.New(), dcWg: newDcWaitGroup(), accounts: newAccountsCache()}
	defer func() {
		if err != nil && mw != nil {
			// delete database if any error occurs after creating it
			mw.DeleteDb()
		}
	}()

	keys, err := wallet.NewKeys(mnemonic)
	if err != nil {
		return
	}

	tb, err := config.GetTrustBase()
	if err != nil {
		return nil, fmt.Errorf("invalid trust base: %w", err)
	}

	mw.Wallet, err = wallet.New(
		wallet.WithBlockProcessor(mw),
		wallet.WithTrustBase(tb),
		wallet.WithAlphabillClientConfig(config.AlphabillClientConfig),
	)
	if err != nil {
		return
	}

	err = saveKeys(db, keys, config.WalletPass)
	if err != nil {
		return
	}

	mw.accounts.add(&account{
		accountIndex: 0,
		accountKeys:  *keys.AccountKey.PubKeyHash,
	})
	return
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

// getBillIds returns billIds from dcMetadata, or parses ids from bills directly
func getBillIds(dbTx TxContext, accountIndex uint64, v *dcBillGroup) ([][]byte, error) {
	dcMeta, err := dbTx.GetDcMetadata(accountIndex, v.dcNonce)
	if err != nil {
		return nil, err
	}
	var billIds [][]byte
	if dcMeta != nil {
		billIds = dcMeta.BillIds
	} else {
		for _, dcBill := range v.dcBills {
			billIds = append(billIds, dcBill.GetID())
		}
	}
	return billIds, nil
}

// groupDcBills groups bills together by dc nonce
func groupDcBills(bills []*Bill) map[uint256.Int]*dcBillGroup {
	m := map[uint256.Int]*dcBillGroup{}
	for _, b := range bills {
		if b.IsDcBill {
			k := *uint256.NewInt(0).SetBytes(b.DcNonce)
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

func validateBlockNumber(blockNumber uint64, lastBlockNumber uint64) error {
	// verify that we are processing blocks sequentially
	// TODO verify last prev block hash?
	if blockNumber != lastBlockNumber+1 {
		return fmt.Errorf("invalid block height. Received blockNumber %d current wallet blockNumber %d", blockNumber, lastBlockNumber)
	}
	return nil
}

func getDb(config WalletConfig, create bool) (Db, error) {
	if config.Db != nil {
		return config.Db, nil
	}
	if create {
		return createNewDb(config)
	}
	return OpenDb(config)
}

func saveKeys(db Db, keys *wallet.Keys, walletPass string) error {
	return db.WithTransaction(func(tx TxContext) error {
		err := tx.SetEncrypted(walletPass != "")
		if err != nil {
			return err
		}
		err = tx.SetMnemonic(keys.Mnemonic)
		if err != nil {
			return err
		}
		err = tx.SetMasterKey(keys.MasterKey.String())
		if err != nil {
			return err
		}
		err = tx.AddAccount(0, keys.AccountKey)
		if err != nil {
			return err
		}
		return tx.SetMaxAccountIndex(0)
	})
}

func (w *Wallet) GetConfig() WalletConfig {
	return w.config
}
