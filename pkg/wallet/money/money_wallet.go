package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
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
	ErrInvalidPassword      = errors.New("invalid password")
	ErrInvalidBlockSystemID = errors.New("invalid system identifier")
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

	mw.Wallet = wallet.New().
		SetBlockProcessor(mw).
		SetABClientConf(config.AlphabillClientConfig).
		Build()

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
	w.db.DeleteDb()
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

// GetBalance returns sum value of all bills currently owned by the wallet, for given account
// the value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalance(accountIndex uint64) (uint64, error) {
	return w.db.Do().GetBalance(accountIndex)
}

// GetBalances returns sum value of all bills currently owned by the wallet, for all accounts
// the value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalances() ([]uint64, error) {
	return w.db.Do().GetBalances()
}

// GetPublicKey returns public key of the wallet (compressed secp256k1 key 33 bytes)
func (w *Wallet) GetPublicKey(accountIndex uint64) ([]byte, error) {
	key, err := w.db.Do().GetAccountKey(accountIndex)
	if err != nil {
		return nil, err
	}
	return key.PubKey, nil
}

// GetPublicKeys returns public keys of the wallet, indexed by account indexes
func (w *Wallet) GetPublicKeys() ([][]byte, error) {
	accKeys, err := w.db.Do().GetAccountKeys()
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
func (w *Wallet) Send(receiverPubKey []byte, amount uint64, accountIndex uint64) error {
	if len(receiverPubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
		return ErrInvalidPubKey
	}

	swapInProgress, err := w.isSwapInProgress(w.db.Do(), accountIndex)
	if err != nil {
		return err
	}
	if swapInProgress {
		return ErrSwapInProgress
	}

	balance, err := w.GetBalance(accountIndex)
	if err != nil {
		return err
	}
	if amount > balance {
		return ErrInsufficientBalance
	}

	maxBlockNo, err := w.GetMaxBlockNumber()
	if err != nil {
		return err
	}
	timeout := maxBlockNo + txTimeoutBlockCount
	if err != nil {
		return err
	}

	k, err := w.db.Do().GetAccountKey(accountIndex)
	if err != nil {
		return err
	}

	bills, err := w.db.Do().GetBills(accountIndex)
	if err != nil {
		return err
	}

	txs, err := createTransactions(receiverPubKey, amount, bills, k, timeout)
	if err != nil {
		return err
	}
	for _, tx := range txs {
		res, err := w.SendTransaction(tx)
		if err != nil {
			return err
		}
		if !res.Ok {
			return errors.New("payment returned error code: " + res.Message)
		}
	}
	return nil
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

// Sync synchronises wallet from the last known block number with the given alphabill node.
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
	switch tx := stx.(type) {
	case money.Transfer:
		if wallet.VerifyP2PKHOwner(&acc.accountKeys, tx.NewBearer()) {
			log.Info("received transfer order")
			err := w.saveWithProof(dbTx, b, &bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				Tx:     txPb,
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
			err := w.saveWithProof(dbTx, b, &bill{
				Id:                  tx.UnitID(),
				Value:               tx.TargetValue(),
				Tx:                  txPb,
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
			err := w.saveWithProof(dbTx, b, &bill{
				Id:     tx.UnitID(),
				Value:  tx.RemainingValue(),
				Tx:     txPb,
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.accountIndex)
			if err != nil {
				return err
			}
		}
		if wallet.VerifyP2PKHOwner(&acc.accountKeys, tx.TargetBearer()) {
			log.Info("received split order (new bill)")
			err := w.saveWithProof(dbTx, b, &bill{
				Id:     util.SameShardId(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256)),
				Value:  tx.Amount(),
				Tx:     txPb,
				TxHash: tx.Hash(crypto.SHA256),
			}, acc.accountIndex)
			if err != nil {
				return err
			}
		}
	case money.Swap:
		if wallet.VerifyP2PKHOwner(&acc.accountKeys, tx.OwnerCondition()) {
			log.Info("received swap order")
			err := w.saveWithProof(dbTx, b, &bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				Tx:     txPb,
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

func (w *Wallet) saveWithProof(dbTx TxContext, b *block.Block, bi *bill, accountIndex uint64) error {
	genericBlock, err := b.ToGenericBlock(txConverter)
	if err != nil {
		return err
	}
	blockProof, err := block.NewPrimaryProof(genericBlock, bi.Id, crypto.SHA256)
	if err != nil {
		return err
	}
	bi.BlockProof = blockProof
	return dbTx.SetBill(accountIndex, bi)
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
			err = w.swapDcBills(tx, billGroup.dcBills, billGroup.dcNonce, timeout, accountIndex)
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
					err = w.swapDcBills(dbTx, v.dcBills, v.dcNonce, swapTimeout, accountIndex)
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
			for _, b := range bills {
				dcValueSum += b.Value
				tx, err := createDustTx(k, b, dcNonce, dcTimeout)
				if err != nil {
					return err
				}

				log.Info("sending dust transfer tx for bill=", b.Id, " account=", accountIndex)
				res, err := w.SendTransaction(tx)
				if err != nil {
					return err
				}
				if !res.Ok {
					return errors.New("dust transfer returned error code: " + res.Message)
				}
			}
			expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: dcNonce, timeout: dcTimeout})
			err = dbTx.SetDcMetadata(accountIndex, dcNonce, &dcMetadata{
				DcValueSum: dcValueSum,
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

func (w *Wallet) swapDcBills(tx TxContext, dcBills []*bill, dcNonce []byte, timeout uint64, accountIndex uint64) error {
	k, err := tx.GetAccountKey(accountIndex)
	if err != nil {
		return err
	}
	swap, err := createSwapTx(k, dcBills, dcNonce, timeout)
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("sending swap tx: nonce=%s timeout=%d", hexutil.Encode(dcNonce), timeout))
	res, err := w.SendTransaction(swap)
	if err != nil {
		return err
	}
	if !res.Ok {
		return errors.New("swap tx returned error code: " + res.Message)
	}
	return tx.SetDcMetadata(accountIndex, dcNonce, &dcMetadata{SwapTimeout: timeout})
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

func createMoneyWallet(config WalletConfig, db Db, mnemonic string) (mw *Wallet, err error) {
	mw = &Wallet{config: config, db: db, dustCollectorJob: cron.New(), dcWg: newDcWaitGroup(), accounts: newAccountsCache()}
	defer func() {
		if err != nil {
			// delete database if any error occurs after creating it
			mw.DeleteDb()
		}
	}()

	keys, err := wallet.NewKeys(mnemonic)
	if err != nil {
		return
	}

	mw.Wallet = wallet.New().
		SetBlockProcessor(mw).
		SetABClientConf(config.AlphabillClientConfig).
		Build()

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

func calculateDcNonce(bills []*bill) []byte {
	var billIds [][]byte
	for _, b := range bills {
		billIds = append(billIds, b.getId())
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
func groupDcBills(bills []*bill) map[uint256.Int]*dcBillGroup {
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
		return errors.New(fmt.Sprintf("Invalid block height. Received blockNumber %d current wallet blockNumber %d", blockNumber, lastBlockNumber))
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
