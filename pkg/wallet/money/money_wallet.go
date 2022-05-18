package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	billtx "gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/money"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/util"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/robfig/cron/v3"
)

const (
	dcTimeoutBlockCount     = 10
	swapTimeoutBlockCount   = 60
	txTimeoutBlockCount     = 100
	dustBillDeletionTimeout = 300
)

var (
	ErrSwapInProgress      = errors.New("swap is in progress, please wait for swap process to be completed before attempting to send transactions")
	ErrInsufficientBalance = errors.New("insufficient balance for transaction")
	ErrInvalidPubKey       = errors.New("invalid public key, public key must be in compressed secp256k1 format")
)

type Wallet struct {
	*wallet.Wallet

	db               Db
	dustCollectorJob *cron.Cron
	dcWg             *dcWaitGroup
}

func CreateNewWallet(config WalletConfig) (*Wallet, error) {
	db, err := getDb(config, true)
	if err != nil {
		return nil, err
	}
	mw := &Wallet{
		db:               db,
		dustCollectorJob: cron.New(),
		dcWg:             newDcWaitGroup(),
	}
	gw, err := wallet.CreateNewWallet(
		db.Do(),
		mw,
		wallet.Config{
			WalletPass:            config.WalletPass,
			AlphabillClientConfig: config.AlphabillClientConfig,
		},
	)
	if err != nil {
		return nil, err
	}
	mw.Wallet = gw
	return mw, nil
}

func CreateNewWalletFromSeed(mnemonic string, config WalletConfig) (*Wallet, error) {
	db, err := getDb(config, true)
	if err != nil {
		return nil, err
	}
	mw := &Wallet{
		db:               db,
		dustCollectorJob: cron.New(),
		dcWg:             newDcWaitGroup(),
	}
	gw, err := wallet.CreateWalletFromSeed(
		db.Do(),
		mw,
		mnemonic,
		wallet.Config{
			WalletPass:            config.WalletPass,
			AlphabillClientConfig: config.AlphabillClientConfig,
		},
	)
	if err != nil {
		return nil, err
	}
	mw.Wallet = gw
	return mw, nil
}

func LoadExistingWallet(config WalletConfig) (*Wallet, error) {
	db, err := getDb(config, false)
	if err != nil {
		return nil, err
	}
	mw := &Wallet{
		db:               db,
		dustCollectorJob: cron.New(),
		dcWg:             newDcWaitGroup(),
	}
	gw, err := wallet.LoadExistingWallet(
		db.Do(),
		mw,
		wallet.Config{
			WalletPass:            config.WalletPass,
			AlphabillClientConfig: config.AlphabillClientConfig,
		},
	)
	if err != nil {
		return nil, err
	}
	mw.Wallet = gw
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

func getDb(config WalletConfig, create bool) (Db, error) {
	if config.Db != nil {
		return config.Db, nil
	}
	if create {
		return createNewDb(config)
	}
	return OpenDb(config)
}

// TODO add walletdb memory layer: https://guardtime.atlassian.net/browse/AB-100
func (w *Wallet) ProcessBlock(b *block.Block) error {
	return w.db.WithTransaction(func(tx TxContext) error {
		log.Info("processing block: " + strconv.FormatUint(b.BlockNumber, 10))
		blockHeight, err := tx.GetBlockHeight()
		if err != nil {
			return err
		}
		err = validateBlockHeight(b, blockHeight)
		if err != nil {
			return err
		}
		for _, txPb := range b.Transactions {
			err = w.collectBills(tx, txPb, blockHeight)
			if err != nil {
				return err
			}
		}
		err = w.deleteExpiredDcBills(tx, b.BlockNumber)
		if err != nil {
			return err
		}
		err = tx.SetBlockHeight(b.BlockNumber)
		if err != nil {
			return err
		}
		err = w.trySwap(tx)
		if err != nil {
			return err
		}
		return nil
	})
}

// postProcessBlock called after successful commit on block processing
func (w *Wallet) PostProcessBlock(b *block.Block) error {
	return w.dcWg.DecrementSwaps(b.BlockNumber, w.db)
}

// Shutdown terminates connection to alphabill node, closes wallet db, cancels dust collector job and any background goroutines.
func (w *Wallet) Shutdown() {
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

// CollectDust starts the dust collector process.
// PartitionWallet needs to be synchronizing using Sync or SyncToMaxBlockHeight in order to receive transactions and finish the process.
// The function blocks until dust collector process is finished or timed out.
func (w *Wallet) CollectDust() error {
	return w.collectDust(true)
}

// StartDustCollectorJob starts the dust collector background process that runs every hour until wallet is shut down.
// PartitionWallet needs to be synchronizing using Sync or SyncToMaxBlockHeight in order to receive transactions and finish the process.
// Returns error if the job failed to start.
func (w *Wallet) StartDustCollectorJob() error {
	_, err := w.startDustCollectorJob()
	return err
}

// GetBalance returns sum value of all bills currently owned by the wallet,
// the value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalance() (uint64, error) {
	return w.db.Do().GetBalance()
}

// Send creates, signs and broadcasts a transaction of the given amount (in the smallest denomination of alphabills)
// to the given public key, the public key must be in compressed secp256k1 format.
func (w *Wallet) Send(pubKey []byte, amount uint64) error {
	if len(pubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
		return ErrInvalidPubKey
	}

	swapInProgress, err := w.isSwapInProgress(w.db.Do())
	if err != nil {
		return err
	}
	if swapInProgress {
		return ErrSwapInProgress
	}

	balance, err := w.GetBalance()
	if err != nil {
		return err
	}
	if amount > balance {
		return ErrInsufficientBalance
	}

	b, err := w.db.Do().GetBillWithMinValue(amount)
	if err != nil {
		return err
	}

	maxBlockNo, err := w.AlphabillClient.GetMaxBlockNo()
	if err != nil {
		return err
	}
	timeout := maxBlockNo + txTimeoutBlockCount
	if err != nil {
		return err
	}

	k, err := w.db.Do().GetAccountKey()
	if err != nil {
		return err
	}
	tx, err := createTransaction(pubKey, k, amount, b, timeout)
	if err != nil {
		return err
	}
	res, err := w.AlphabillClient.SendTransaction(tx)
	if err != nil {
		return err
	}
	if !res.Ok {
		return errors.New("payment returned error code: " + res.Message)
	}
	return nil
}

func (w *Wallet) collectBills(dbTx TxContext, txPb *transaction.Transaction, blockHeight uint64) error {
	gtx, err := billtx.NewMoneyTx(txPb)
	if err != nil {
		return err
	}
	stx := gtx.(transaction.GenericTransaction)

	switch tx := stx.(type) {
	case money.Transfer:
		isOwner, err := w.isOwner(dbTx, tx.NewBearer())
		if err != nil {
			return err
		}
		if isOwner {
			err = dbTx.SetBill(&bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
		} else {
			err := dbTx.RemoveBill(tx.UnitID())
			if err != nil {
				return err
			}
		}
	case money.TransferDC:
		isOwner, err := w.isOwner(dbTx, tx.TargetBearer())
		if err != nil {
			return err
		}
		if isOwner {
			err = dbTx.SetBill(&bill{
				Id:                  tx.UnitID(),
				Value:               tx.TargetValue(),
				TxHash:              tx.Hash(crypto.SHA256),
				IsDcBill:            true,
				DcTx:                txPb,
				DcTimeout:           tx.Timeout(),
				DcNonce:             tx.Nonce(),
				DcExpirationTimeout: blockHeight + dustBillDeletionTimeout,
			})
		} else {
			err := dbTx.RemoveBill(tx.UnitID())
			if err != nil {
				return err
			}
		}
	case money.Split:
		// split tx contains two bills: existing bill and new bill
		// if any of these bills belong to wallet then we have to
		// 1) update the existing bill and
		// 2) add the new bill
		containsBill, err := dbTx.ContainsBill(tx.UnitID())
		if err != nil {
			return err
		}
		if containsBill {
			err := dbTx.SetBill(&bill{
				Id:     tx.UnitID(),
				Value:  tx.RemainingValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
		}
		isOwner, err := w.isOwner(dbTx, tx.TargetBearer())
		if err != nil {
			return err
		}
		if isOwner {
			err := dbTx.SetBill(&bill{
				Id:     util.SameShardId(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256)),
				Value:  tx.Amount(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
		}
	case money.Swap:
		isOwner, err := w.isOwner(dbTx, tx.OwnerCondition())
		if err != nil {
			return err
		}
		if isOwner {
			err = dbTx.SetBill(&bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}

			// clear dc metadata
			err = dbTx.SetDcMetadata(txPb.UnitId, nil)
			if err != nil {
				return err
			}

			for _, dustTransfer := range tx.DCTransfers() {
				err := dbTx.RemoveBill(dustTransfer.UnitID())
				if err != nil {
					return err
				}
			}
		} else {
			err := dbTx.RemoveBill(tx.UnitID())
			if err != nil {
				return err
			}
		}
	default:
		panic(fmt.Sprintf("received unknown transaction: %s", tx))
	}
	return nil
}

func (w *Wallet) deleteExpiredDcBills(dbTx TxContext, blockHeight uint64) error {
	bills, err := dbTx.GetBills()
	if err != nil {
		return err
	}
	for _, b := range bills {
		if b.isExpired(blockHeight) {
			err = dbTx.RemoveBill(b.Id)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Wallet) trySwap(tx TxContext) error {
	blockHeight, err := tx.GetBlockHeight()
	if err != nil {
		return err
	}
	maxBlockNo, err := w.AlphabillClient.GetMaxBlockNo()
	if err != nil {
		return err
	}
	bills, err := tx.GetBills()
	if err != nil {
		return err
	}
	dcBillGroups := groupDcBills(bills)
	for nonce, billGroup := range dcBillGroups {
		nonce32 := nonce.Bytes32()
		dcMeta, err := tx.GetDcMetadata(nonce32[:])
		if err != nil {
			return err
		}
		if dcMeta != nil && dcMeta.isSwapRequired(blockHeight, billGroup.valueSum) {
			timeout := maxBlockNo + swapTimeoutBlockCount
			err := w.swapDcBills(tx, billGroup.dcBills, billGroup.dcNonce, timeout)
			if err != nil {
				return err
			}
			w.dcWg.UpdateTimeout(billGroup.dcNonce, timeout)
		}
	}

	// delete expired metadata
	nonceMetadataMap, err := tx.GetDcMetadataMap()
	if err != nil {
		return err
	}
	for nonce, m := range nonceMetadataMap {
		if m.timeoutReached(blockHeight) {
			nonce32 := nonce.Bytes32()
			err := tx.SetDcMetadata(nonce32[:], nil)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// collectDust sends dust transfer for every bill in wallet and records metadata.
// Once the dust transfers get confirmed on the ledger then swap transfer is broadcast and metadata cleared.
// If blocking is true then the function blocks until swap has been completed or timed out,
// if blocking is false then the function returns after sending the dc transfers.
func (w *Wallet) collectDust(blocking bool) error {
	err := w.db.WithTransaction(func(dbTx TxContext) error {
		blockHeight, err := dbTx.GetBlockHeight()
		if err != nil {
			return err
		}
		maxBlockNo, err := w.AlphabillClient.GetMaxBlockNo()
		if err != nil {
			return err
		}
		bills, err := dbTx.GetBills()
		if err != nil {
			return err
		}
		if len(bills) < 2 {
			return nil
		}
		var expectedSwaps []expectedSwap
		dcBillGroups := groupDcBills(bills)
		if len(dcBillGroups) > 0 {
			for _, v := range dcBillGroups {
				if blockHeight >= v.dcTimeout {
					swapTimeout := maxBlockNo + swapTimeoutBlockCount
					err = w.swapDcBills(dbTx, v.dcBills, v.dcNonce, swapTimeout)
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
			swapInProgress, err := w.isSwapInProgress(dbTx)
			if err != nil {
				return err
			}
			if swapInProgress {
				log.Warning("cannot start dust collection while previous collection round is still in progress")
				return nil
			}

			k, err := dbTx.GetAccountKey()
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

				log.Info("sending dust transfer tx for bill ", b.Id)
				res, err := w.AlphabillClient.SendTransaction(tx)
				if err != nil {
					return err
				}
				if !res.Ok {
					return errors.New("dust transfer returned error code: " + res.Message)
				}
			}
			expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: dcNonce, timeout: dcTimeout})
			err = dbTx.SetDcMetadata(dcNonce, &dcMetadata{
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
		log.Info("waiting for blocking collect dust")
		w.dcWg.wg.Wait()
		log.Info("finished waiting for blocking collect dust")
	}
	return nil
}

// isSwapInProgress returns true if there's a running dc process managed by the wallet
func (w *Wallet) isSwapInProgress(dbTx TxContext) (bool, error) {
	blockHeight, err := dbTx.GetBlockHeight()
	if err != nil {
		return false, err
	}
	dcMetadataMap, err := dbTx.GetDcMetadataMap()
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
		err := w.collectDust(false)
		if err != nil {
			log.Error("error in dust collector job: ", err)
		}
	})
}

func (w *Wallet) swapDcBills(tx TxContext, dcBills []*bill, dcNonce []byte, timeout uint64) error {
	k, err := tx.GetAccountKey()
	if err != nil {
		return err
	}
	swap, err := createSwapTx(k, dcBills, dcNonce, timeout)
	if err != nil {
		return err
	}
	log.Info("sending swap tx")
	res, err := w.AlphabillClient.SendTransaction(swap)
	if err != nil {
		return err
	}
	if !res.Ok {
		return errors.New("swap tx returned error code: " + res.Message)
	}
	return tx.SetDcMetadata(dcNonce, &dcMetadata{SwapTimeout: timeout})
}

// isOwner checks if given p2pkh bearer predicate contains PartitionWallet's pubKey hash
func (w *Wallet) isOwner(dbTx TxContext, bp []byte) (bool, error) {
	// p2pkh predicate: [0x53, 0x76, 0xa8, 0x01, 0x4f, 0x01, <32 bytes>, 0x87, 0x69, 0xac, 0x01]
	// p2pkh predicate: [Dup, Hash <SHA256>, PushHash <SHA256> <32 bytes>, Equal, Verify, CheckSig <secp256k1>]

	// p2pkh owner predicate must be 10 + (32 or 64) (SHA256 or SHA512) bytes long
	if len(bp) != 42 && len(bp) != 74 {
		return false, nil
	}
	// 5th byte is PushHash 0x4f
	if bp[4] != 0x4f {
		return false, nil
	}
	// 6th byte is HashAlgo 0x01 or 0x02 for SHA256 and SHA512 respectively
	hashAlgo := bp[5]
	if hashAlgo == 0x01 {
		k, err := dbTx.GetAccountKey()
		if err != nil {
			return false, err
		}
		return bytes.Equal(bp[6:38], k.PubKeyHashSha256), nil
	} else if hashAlgo == 0x02 {
		k, err := dbTx.GetAccountKey()
		if err != nil {
			return false, err
		}
		return bytes.Equal(bp[6:70], k.PubKeyHashSha512), nil
	}
	return false, nil
}

func validateBlockHeight(b *block.Block, blockHeight uint64) error {
	// verify that we are processing blocks sequentially
	// TODO verify last prev block hash?
	// TODO will genesis block be height 0 or 1: https://guardtime.atlassian.net/browse/AB-101
	if b.BlockNumber-blockHeight != 1 {
		return errors.New(fmt.Sprintf("Invalid block height. Received height %d current wallet height %d", b.BlockNumber, blockHeight))
	}
	return nil
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
