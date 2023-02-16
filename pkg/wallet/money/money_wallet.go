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
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	txverifier "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_verifier"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"
)

const (
	dcTimeoutBlockCount     = 10
	swapTimeoutBlockCount   = 60
	txTimeoutBlockCount     = 100
	dustBillDeletionTimeout = 65536
)

var (
	ErrSwapInProgress               = errors.New("swap is in progress, synchronize your wallet to complete the process")
	ErrInsufficientBalance          = errors.New("insufficient balance for transaction")
	ErrInsufficientFeeCredit        = errors.New("insufficient fee credit balance for transaction(s)")
	ErrInsufficientBillValue        = errors.New("wallet does not have a bill large enough for fee transfer")
	ErrInvalidCreateFeeCreditAmount = errors.New("fee credit amount must be positive")
	ErrInvalidPubKey                = errors.New("invalid public key, public key must be in compressed secp256k1 format")
	ErrInvalidPassword              = errors.New("invalid password")
	ErrInvalidBlockSystemID         = errors.New("invalid system identifier")
	ErrTxFailedToConfirm            = errors.New("transaction(s) failed to confirm")
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
		am               account.Manager
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
func CreateNewWallet(am account.Manager, mnemonic string, config WalletConfig) (*Wallet, error) {
	db, err := getDb(config, true)
	if err != nil {
		return nil, err
	}
	return createMoneyWallet(config, db, mnemonic, am)
}

func LoadExistingWallet(config WalletConfig, am account.Manager) (*Wallet, error) {
	db, err := getDb(config, false)
	if err != nil {
		return nil, err
	}

	mw := &Wallet{config: config, db: db, dustCollectorJob: cron.New(), dcWg: newDcWaitGroup(), am: am}

	mw.Wallet = wallet.New().
		SetBlockProcessor(mw).
		SetABClientConf(config.AlphabillClientConfig).
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

func (w *Wallet) ProcessBlock(b *block.Block) error {
	log.Info("processing block: ", b.UnicityCertificate.InputRecord.RoundNumber)
	if !bytes.Equal(w.SystemID(), b.GetSystemIdentifier()) {
		return ErrInvalidBlockSystemID
	}

	return w.db.WithTransaction(func(dbTx TxContext) error {
		lastBlockNumber, err := dbTx.GetBlockNumber()
		if err != nil {
			return err
		}
		err = validateBlockNumber(b.UnicityCertificate.InputRecord.RoundNumber, lastBlockNumber)
		if err != nil {
			return err
		}
		for _, acc := range w.am.GetAll() {
			fcb, err := dbTx.GetFeeCreditBill(acc.AccountIndex)
			if err != nil {
				return err
			}
			for _, pbTx := range b.Transactions {
				err = w.collectBills(dbTx, pbTx, b, getFCBlockNumber(fcb), &acc)
				if err != nil {
					return err
				}
			}
		}
		return w.endBlock(dbTx, b)
	})
}

func (w *Wallet) endBlock(dbTx TxContext, b *block.Block) error {
	blockNumber := b.UnicityCertificate.InputRecord.RoundNumber
	err := dbTx.SetBlockNumber(blockNumber)
	if err != nil {
		return err
	}
	for _, acc := range w.am.GetAll() {
		err = w.deleteExpiredDcBills(dbTx, blockNumber, acc.AccountIndex)
		if err != nil {
			return err
		}
		err = w.trySwap(dbTx, acc.AccountIndex)
		if err != nil {
			return err
		}
		err = w.dcWg.DecrementSwaps(dbTx, blockNumber, acc.AccountIndex)
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

	w.am.Close()
}

// DeleteDb deletes the wallet database.
func (w *Wallet) DeleteDb() {
	w.db.DeleteDb()
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
	return w.db.Do().GetAllBills(w.am)
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
	key, err := w.am.GetAccountKey(accountIndex)
	if err != nil {
		return err
	}
	gtx, err := NewTxConverter(w.SystemID()).ConvertTx(tx)
	if err != nil {
		return err
	}
	err = txverifier.VerifyTxP2PKHOwner(gtx, key.PubKeyHash)
	if err != nil {
		return err
	}
	return w.db.Do().SetBill(accountIndex, bill)
}

// AddAccount adds the next account in account key series to the wallet.
// New accounts are indexed only from the time of creation and not backwards in time.
// Returns new account's index and public key.
func (w *Wallet) AddAccount() (uint64, []byte, error) {
	idx, pubKey, err := w.am.AddAccount()
	if err != nil {
		return 0, nil, err
	}
	return idx, pubKey, w.db.Do().AddAccount(idx)
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

	k, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	bills, err := w.db.Do().GetBills(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	fcb, err := w.db.Do().GetFeeCreditBill(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	if fcb == nil {
		return nil, ErrInsufficientFeeCredit
	}

	_, roundNumber, err := w.GetMaxBlockNumber()
	if err != nil {
		return nil, err
	}
	timeout := roundNumber + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	txs, err := createTransactions(cmd.ReceiverPubKey, cmd.Amount, w.SystemID(), bills, k, timeout, fcb.GetID())
	if err != nil {
		return nil, err
	}

	txsCost := maxFee * uint64(len(txs))
	if fcb.Value < txsCost {
		return nil, ErrInsufficientFeeCredit
	}

	for _, tx := range txs {
		err := w.SendTransaction(ctx, tx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
		if err != nil {
			return nil, err
		}
	}
	if cmd.WaitForConfirmation {
		txProofs, err := w.waitForConfirmation(ctx, txs, roundNumber, timeout, cmd.AccountIndex)
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
	// must have enough balance to not end up with zero fee credits
	maxTotalFees := 2 * maxFee
	if cmd.Amount+maxTotalFees > balance {
		return nil, ErrInsufficientBalance
	}

	bills, err := w.db.Do().GetBills(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})
	billToTransfer := bills[0]

	if billToTransfer.Value < cmd.Amount+maxTotalFees {
		return nil, ErrInsufficientBillValue
	}

	k, err := w.db.Do().GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	fcb, err := w.db.Do().GetFeeCreditBill(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	_, lastRoundNo, err := w.GetMaxBlockNumber()
	if err != nil {
		return nil, err
	}
	timeout := lastRoundNo + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	log.Info("sending transfer fee credit transaction")
	tx, err := createTransferFCTx(cmd.Amount, k.PrivKeyHash, getTxHash(fcb), k, w.SystemID(), billToTransfer, lastRoundNo, timeout)
	if err != nil {
		return nil, err
	}
	err = w.SendTransaction(ctx, tx, &wallet.SendOpts{})
	if err != nil {
		return nil, err
	}
	// TODO waiting can be canceled or AddFC tx can fail
	// record some metadata about transferFC so that AddFC can be retried
	fcTransferProof, err := w.waitForConfirmation(ctx, []*txsystem.Transaction{tx}, lastRoundNo, timeout, cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	log.Info("sending add fee credit transaction")
	addFCTx, err := createAddFCTx(k.PrivKeyHash, fcTransferProof[0], k, w.SystemID(), timeout)
	if err != nil {
		return nil, err
	}
	err = w.SendTransaction(ctx, addFCTx, &wallet.SendOpts{})
	if err != nil {
		return nil, err
	}
	return w.waitForConfirmation(ctx, []*txsystem.Transaction{addFCTx}, lastRoundNo, timeout, cmd.AccountIndex)
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns list of "reclaim fee credit" transaction proofs.
func (w *Wallet) ReclaimFeeCredit(ctx context.Context, cmd ReclaimFeeCmd) ([]*BlockProof, error) {
	k, err := w.db.Do().GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	_, lastRoundNo, err := w.GetMaxBlockNumber()
	if err != nil {
		return nil, err
	}
	timeout := lastRoundNo + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	fcb, err := w.db.Do().GetFeeCreditBill(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	if fcb == nil || fcb.Value == 0 {
		return nil, ErrInsufficientFeeCredit
	}

	bills, err := w.db.Do().GetBills(cmd.AccountIndex)
	if err != nil {
		return nil, err
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
	tx, err := createCloseFCTx(w.SystemID(), fcb.GetID(), timeout, fcb.Value, targetBill.GetID(), targetBill.TxHash, k)
	if err != nil {
		return nil, err
	}
	err = w.SendTransaction(ctx, tx, &wallet.SendOpts{})
	if err != nil {
		return nil, err
	}
	fcTransferProof, err := w.waitForConfirmation(ctx, []*txsystem.Transaction{tx}, lastRoundNo, timeout, cmd.AccountIndex)
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
	return w.waitForConfirmation(ctx, []*txsystem.Transaction{addFCTx}, lastRoundNo, timeout, cmd.AccountIndex)
}

// GetFeeCreditBill returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCreditBill(accountIndex uint64) (*Bill, error) {
	return w.db.Do().GetFeeCreditBill(accountIndex)
}

// GetMaxAccountIndex returns last added (largest) account number.
func (w *Wallet) GetMaxAccountIndex() (uint64, error) {
	return w.db.Do().GetMaxAccountIndex()
}

func (w *Wallet) GetConfig() WalletConfig {
	return w.config
}

func (w *Wallet) collectBills(dbTx TxContext, txPb *txsystem.Transaction, b *block.Block, fcBlockNumber uint64, acc *account.Account) error {
	gtx, err := money.NewMoneyTx(w.SystemID(), txPb)
	if err != nil {
		return err
	}
	currentBlockNumber := b.GetRoundNumber()
	switch tx := gtx.(type) {
	case money.Transfer:
		err := w.updateFCB(dbTx, txPb, acc, fcBlockNumber, currentBlockNumber)
		if err != nil {
			return err
		}
		if account.VerifyP2PKHOwner(&acc.AccountKeys, tx.NewBearer()) {
			bill, err := dbTx.GetBill(acc.AccountIndex, txPb.UnitId)
			if err != nil && !errors.Is(err, errBillNotFound) {
				return err
			}
			if bill != nil && bill.BlockProof.BlockNumber >= currentBlockNumber {
				log.Debug("received transfer order (already processed)")
				return nil
			}
			log.Info("received transfer order")
			return w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.AccountIndex)
		} else {
			return dbTx.RemoveBill(acc.AccountIndex, tx.UnitID())
		}
	case money.TransferDC:
		if account.VerifyP2PKHOwner(&acc.AccountIndex, tx.TargetBearer()) {
			bill, err := dbTx.GetBill(acc.AccountIndex, txPb.UnitId)
			if err != nil && !errors.Is(err, errBillNotFound) {
				return err
			}
			if bill != nil && bill.BlockProof.BlockNumber >= currentBlockNumber {
				log.Debug("received transferDC order (already processed)")
				return nil
			}
			log.Info("received TransferDC order")
			return w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:                  tx.UnitID(),
				Value:               tx.TargetValue(),
				TxHash:              tx.Hash(crypto.SHA256),
				IsDcBill:            true,
				DcTimeout:           tx.Timeout(),
				DcNonce:             tx.Nonce(),
				DcExpirationTimeout: b.UnicityCertificate.InputRecord.RoundNumber + dustBillDeletionTimeout,
			}, acc.AccountIndex)
		} else {
			return dbTx.RemoveBill(acc.AccountIndex, tx.UnitID())
		}
	case money.Split:
		err := w.updateFCB(dbTx, txPb, acc, fcBlockNumber, currentBlockNumber)
		if err != nil {
			return err
		}
		// split tx contains two bills: existing bill and new bill
		// if any of these bills belong to wallet then we have to
		// 1) update the existing bill and
		// 2) add the new bill
		oldBill, err := dbTx.GetBill(acc.AccountIndex, txPb.UnitId)
		if err != nil && !errors.Is(err, errBillNotFound) {
			return err
		}
		if oldBill != nil {
			if oldBill.BlockProof.BlockNumber >= currentBlockNumber {
				log.Debug("received split order (existing bill) (already processed)")
				return nil
			}
			log.Info("received split order (existing bill)")
			err := w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:     tx.UnitID(),
				Value:  tx.RemainingValue(),
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.AccountIndex)
			if err != nil {
				return err
			}
		}
		if wallet.VerifyP2PKHOwner(&acc.AccountKeys, tx.TargetBearer()) {
			newUnitID := util.SameShardIDBytes(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256))
			newUnit, err := dbTx.GetBill(acc.AccountIndex, newUnitID)
			if err != nil && !errors.Is(err, errBillNotFound) {
				return err
			}
			if newUnit != nil && newUnit.BlockProof.BlockNumber >= currentBlockNumber {
				log.Debug("received split order (new bill) (already processed)")
				return nil
			}
			log.Info("received split order (new bill)")
			return w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:     uint256.NewInt(0).SetBytes(newUnitID),
				Value:  tx.Amount(),
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.AccountIndex)
		}
	case money.Swap:
		err := w.updateFCB(dbTx, txPb, acc, fcBlockNumber, currentBlockNumber)
		if err != nil {
			return err
		}
		if account.VerifyP2PKHOwner(&acc.AccountKeys, tx.OwnerCondition()) {
			bill, err := dbTx.GetBill(acc.AccountIndex, txPb.UnitId)
			if err != nil && !errors.Is(err, errBillNotFound) {
				return err
			}
			if bill != nil && bill.BlockProof.BlockNumber >= currentBlockNumber {
				log.Debug("received swap order (already processed)")
				return nil
			}
			log.Info("received swap order")
			err = w.saveWithProof(dbTx, b, txPb, &Bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.AccountIndex)
			if err != nil {
				return err
			}
			// clear dc metadata
			err = dbTx.SetDcMetadata(acc.AccountIndex, txPb.UnitId, nil)
			if err != nil {
				return err
			}
			for _, dustTransfer := range tx.DCTransfers() {
				err := dbTx.RemoveBill(acc.AccountIndex, dustTransfer.UnitID())
				if err != nil {
					return err
				}
			}
		} else {
			return dbTx.RemoveBill(acc.AccountIndex, tx.UnitID())
		}
	case *fc.TransferFeeCreditWrapper:
		bill, err := dbTx.GetBill(acc.AccountIndex, tx.Transaction.UnitId)
		if err != nil && !errors.Is(err, errBillNotFound) {
			return err
		}
		if bill == nil {
			return nil
		}
		if bill.BlockProof.BlockNumber >= currentBlockNumber {
			log.Debug("received transferFC order (already processed)")
			return nil
		}
		log.Info("received transferFC order")
		return w.saveWithProof(dbTx, b, txPb, &Bill{
			Id:     bill.Id,
			Value:  bill.Value - tx.TransferFC.Amount - tx.Transaction.ServerMetadata.Fee,
			TxHash: tx.Hash(crypto.SHA256),
		}, acc.AccountIndex)
	case *fc.AddFeeCreditWrapper:
		if bytes.Equal(txPb.UnitId, acc.privKeyHash) {
			fcb, err := dbTx.GetFeeCreditBill(acc.AccountIndex)
			if err != nil {
				return err
			}
			if fcb != nil && fcb.BlockProof.BlockNumber >= currentBlockNumber {
				log.Debug("received addFC order (already processed)")
				return nil
			}
			log.Info("received addFC order")
			err = w.saveFCBWithProof(dbTx, b, txPb, &Bill{
				Id:            tx.UnitID(),
				Value:         getValue(fcb) + tx.TransferFC.TransferFC.Amount - tx.Transaction.ServerMetadata.Fee,
				TxHash:        tx.Hash(crypto.SHA256),
				FCBlockNumber: currentBlockNumber,
			}, acc.AccountIndex)
		}
		return nil
	case *fc.CloseFeeCreditWrapper:
		if bytes.Equal(txPb.UnitId, acc.privKeyHash) {
			fcb, err := dbTx.GetFeeCreditBill(acc.AccountIndex)
			if err != nil {
				return err
			}
			if fcb != nil && fcb.BlockProof.BlockNumber >= currentBlockNumber {
				log.Debug("received closeFC order (already processed)")
				return nil
			}
			log.Info("received closeFC order")
			err = w.saveFCBWithProof(dbTx, b, txPb, &Bill{
				Id:            tx.UnitID(),
				Value:         getValue(fcb) - tx.CloseFC.Amount,
				TxHash:        tx.Hash(crypto.SHA256),
				FCBlockNumber: currentBlockNumber,
			}, acc.AccountIndex)
			if err != nil {
				return err
			}
		}
		return nil
	case *fc.ReclaimFeeCreditWrapper:
		bill, err := dbTx.GetBill(acc.AccountIndex, tx.Transaction.UnitId)
		if err != nil && !errors.Is(err, errBillNotFound) {
			return err
		}
		if bill == nil {
			return nil
		}
		if bill.BlockProof.BlockNumber >= currentBlockNumber {
			log.Debug("received transferFC order (already processed)")
			return nil
		}
		log.Info("received reclaimFC order")
		reclaimedValue := tx.CloseFCTransfer.CloseFC.Amount - tx.CloseFCTransfer.Transaction.ServerMetadata.Fee - tx.Transaction.ServerMetadata.Fee
		return w.saveWithProof(dbTx, b, txPb, &Bill{
			Id:     bill.Id,
			Value:  bill.Value + reclaimedValue,
			TxHash: tx.Hash(crypto.SHA256),
		}, acc.AccountIndex)
	default:
		log.Warning(fmt.Sprintf("received unknown transaction type, skipping processing: %s", tx))
		return nil
	}
	return nil
}

func (w *Wallet) updateFCB(dbTx TxContext, txPb *txsystem.Transaction, acc *account, fcBlockNumber, currentBlockNumber uint64) error {
	if bytes.Equal(txPb.ClientMetadata.FeeCreditRecordId, acc.privKeyHash) {
		fcb, err := dbTx.GetFeeCreditBill(acc.AccountIndex)
		if err != nil {
			return err
		}
		if fcb == nil {
			return errors.New("received tx with wallet's fee credit record id, but wallet's FCB is nil")
		}
		if fcBlockNumber >= currentBlockNumber {
			log.Debug("not updating FCB (already updated)")
			return nil
		}
		if fcb.Value < txPb.ServerMetadata.Fee {
			return errors.New("received tx where charged fee is greater than wallet's FCB balance")
		}
		log.Debug("updating FCB value: old=", fcb.Value, " new=", fcb.Value-txPb.ServerMetadata.Fee, " lastUpdatedBlockNumber=", fcBlockNumber, " CurrentBlockNumber=", currentBlockNumber)
		fcb.Value -= txPb.ServerMetadata.Fee
		fcb.FCBlockNumber = currentBlockNumber
		err = dbTx.SetFeeCreditBill(acc.AccountIndex, fcb)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) saveWithProof(dbTx TxContext, b *block.Block, txPb *txsystem.Transaction, bill *Bill, accountIndex uint64) error {
	err := bill.addProof(b, txPb, NewTxConverter(w.SystemID()))
	if err != nil {
		return err
	}
	return dbTx.SetBill(accountIndex, bill)
}

func (w *Wallet) saveFCBWithProof(dbTx TxContext, b *block.Block, txPb *txsystem.Transaction, fcb *Bill, accountIndex uint64) error {
	err := fcb.addProof(b, txPb, NewTxConverter(w.SystemID()))
	if err != nil {
		return err
	}
	return dbTx.SetFeeCreditBill(accountIndex, fcb)
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
			_, maxRoundNumber, err := w.GetMaxBlockNumber()
			if err != nil {
				return err
			}
			timeout := maxRoundNumber + swapTimeoutBlockCount
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
		_, maxRoundNumber, err := w.GetMaxBlockNumber()
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
					swapTimeout := maxRoundNumber + swapTimeoutBlockCount
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

			fcb, err := dbTx.GetFeeCreditBill(accountIndex)
			if err != nil {
				return err
			}
			if fcb == nil || fcb.Value < maxFee*uint64(len(bills)) {
				return ErrInsufficientFeeCredit
			}

			k, err := w.am.GetAccountKey(accountIndex)
			if err != nil {
				return err
			}

			dcNonce := calculateDcNonce(bills)
			dcTimeout := maxRoundNumber + dcTimeoutBlockCount
			var dcValueSum uint64
			var billIds [][]byte
			for _, b := range bills {
				dcValueSum += b.Value
				billIds = append(billIds, b.GetID())
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
	k, err := w.am.GetAccountKey(accountIndex)
	if err != nil {
		return err
	}
	fcb, err := tx.GetFeeCreditBill(accountIndex)
	if err != nil {
		return err
	}
	if fcb == nil || fcb.Value < maxFee {
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
		for _, acc := range w.am.GetAll() {
			err := w.collectDust(context.Background(), false, acc.AccountIndex)
			if err != nil {
				log.Error("error in dust collector job: ", err)
			}
		}
	})
}

func (w *Wallet) waitForConfirmation(ctx context.Context, pendingTxs []*txsystem.Transaction, latestRoundNumber, timeout, accountIndex uint64) ([]*BlockProof, error) {
	log.Info("waiting for confirmation(s)...")
	latestBlockNumber := latestRoundNumber
	txsLog := newTxLog(pendingTxs)
	txc := NewTxConverter(w.SystemID())
	for latestBlockNumber <= timeout {
		b, err := w.AlphabillClient.GetBlock(latestBlockNumber)
		if err != nil {
			return nil, err
		}
		if b == nil {
			// block might be empty, check latest round number
			_, latestRoundNumber, err = w.AlphabillClient.GetMaxBlockNumber()
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
		genericBlock, err := b.ToGenericBlock(txc)
		if err != nil {
			return nil, err
		}
		for _, gtx := range genericBlock.Transactions {
			tx := gtx.ToProtoBuf()
			if txsLog.contains(tx) {
				log.Info("confirmed tx ", hexutil.Encode(tx.UnitId))
				err = w.db.WithTransaction(func(dbTx TxContext) error {
					fcb, err := dbTx.GetFeeCreditBill(accountIndex)
					if err != nil {
						return err
					}
					return w.collectBills(dbTx, tx, b, getFCBlockNumber(fcb), &w.am.GetAll()[accountIndex])
				})
				if err != nil {
					return nil, err
				}
				err = txsLog.recordTx(gtx, genericBlock)
				if err != nil {
					return nil, err
				}
				if txsLog.isAllTxsConfirmed() {
					log.Info("transaction(s) confirmed")
					return txsLog.getAllRecordedProofs(), nil
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

func createMoneyWallet(config WalletConfig, db Db, mnemonic string, am account.Manager) (mw *Wallet, err error) {
	mw = &Wallet{config: config, db: db, dustCollectorJob: cron.New(), dcWg: newDcWaitGroup(), am: am}
	defer func() {
		if err != nil {
			// delete database if any error occurs after creating it
			mw.DeleteDb()
		}
	}()

	err = am.CreateKeys(mnemonic)
	if err != nil {
		return
	}

	err = mw.db.Do().AddAccount(0)
	if err != nil {
		return
	}

	mw.Wallet = wallet.New().
		SetBlockProcessor(mw).
		SetABClientConf(config.AlphabillClientConfig).
		Build()

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
	// TODO verify last prev block hash?
	// TODO: AB-505 block numbers are not sequential any more, gaps might appear as empty block are not stored and sent
	if lastBlockNumber >= blockNumber {
		return fmt.Errorf("invalid block number. Received blockNumber %d current wallet blockNumber %d", blockNumber, lastBlockNumber)
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
	return openDb(config)
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
			Id:         util.SameShardID(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256)),
			Value:      tx.Amount(),
			TxHash:     tx.Hash(crypto.SHA256),
			BlockProof: proof,
		}, nil
	default:
		return nil, errors.New("cannot convert unsupported tx type to Bill struct")
	}
}
