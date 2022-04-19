package wallet

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/abclient"
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/money"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/util"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/holiman/uint256"
	"github.com/robfig/cron/v3"
	"github.com/tyler-smith/go-bip39"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	prefetchBlockCount          = 10
	dcTimeoutBlockCount         = 10
	swapTimeoutBlockCount       = 60
	txTimeoutBlockCount         = 100
	dustBillDeletionTimeout     = 300
	mnemonicEntropyBitSize      = 128
	sleepTimeAtMaxBlockHeightMs = 500
)

var (
	ErrInvalidPubKey       = errors.New("invalid public key, public key must be in compressed secp256k1 format")
	ErrSwapInProgress      = errors.New("swap is in progress, please wait for swap process to be completed before attempting to send transactions")
	ErrInsufficientBalance = errors.New("insufficient balance for transaction")
	ErrInvalidPassword     = errors.New("invalid password")
)

type Wallet struct {
	config           Config
	db               Db
	alphaBillClient  abclient.ABClient
	dustCollectorJob *cron.Cron
	dcWg             dcWaitGroup
	syncFlag         *syncFlagWrapper
}

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func CreateNewWallet(config Config) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}

	mnemonic, err := generateMnemonic()
	if err != nil {
		return nil, err
	}
	return createWallet(mnemonic, config)
}

// CreateWalletFromSeed creates a new wallet from given seed mnemonic. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func CreateWalletFromSeed(mnemonic string, config Config) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}
	return createWallet(mnemonic, config)
}

// LoadExistingWallet loads an existing wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func LoadExistingWallet(config Config) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}

	db, err := getDb(config, false)
	if err != nil {
		return nil, err
	}

	ok, err := db.VerifyPassword()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidPassword
	}

	return newWallet(config, db)
}

// GetBalance returns sum value of all bills currently owned by the wallet,
// the value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalance() (uint64, error) {
	return w.db.GetBalance()
}

// GetPublicKey returns public key of the wallet (compressed secp256k1 key 33 bytes)
func (w *Wallet) GetPublicKey() ([]byte, error) {
	key, err := w.db.GetAccountKey()
	if err != nil {
		return nil, err
	}
	return key.PubKey, err
}

// Send creates, signs and broadcasts a transaction of the given amount (in the smallest denomination of alphabills)
// to the given public key, the public key must be in compressed secp256k1 format.
func (w *Wallet) Send(pubKey []byte, amount uint64) error {
	if len(pubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
		return ErrInvalidPubKey
	}

	swapInProgress, err := w.isSwapInProgress()
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

	b, err := w.db.GetBillWithMinValue(amount)
	if err != nil {
		return err
	}

	maxBlockNo, err := w.alphaBillClient.GetMaxBlockNo()
	if err != nil {
		return err
	}
	timeout := maxBlockNo + txTimeoutBlockCount
	if err != nil {
		return err
	}

	k, err := w.db.GetAccountKey()
	if err != nil {
		return err
	}
	tx, err := createTransaction(pubKey, k, amount, b, timeout)
	if err != nil {
		return err
	}
	res, err := w.alphaBillClient.SendTransaction(tx)
	if err != nil {
		return err
	}
	if !res.Ok {
		return errors.New("payment returned error code: " + res.Message)
	}
	return nil
}

// Sync synchronises wallet with given alphabill node and starts dust collector background job,
// it blocks forever or until alphabill connection is terminated. This function should only be called ONCE.
func (w *Wallet) Sync() error {
	_, err := w.startDustCollectorJob()
	if err != nil {
		return err
	}
	return w.syncLedger(true)
}

// SyncToMaxBlockHeight synchronises wallet with given alphabill node, it blocks until maximum block height,
// at the start of the process, is reached. It does not start dust collector background process.
func (w *Wallet) SyncToMaxBlockHeight() error {
	return w.syncLedger(false)
}

// CollectDust starts the dust collector process,
// it blocks until dust collector process is finished or times out.
func (w *Wallet) CollectDust() error {
	go func() {
		err := w.syncLedger(true)
		if err != nil {
			log.Error("error syncronizing ledger for CollectDust %w", err)
		}
	}()
	return w.collectDust(true)
}

// Shutdown terminates connection to alphabill node, closes wallet db and any background goroutines.
func (w *Wallet) Shutdown() {
	log.Info("Shutting down wallet")
	if w.alphaBillClient != nil {
		w.alphaBillClient.Shutdown()
	}
	if w.dustCollectorJob != nil {
		w.dustCollectorJob.Stop()
	}
	w.dcWg.ResetWaitGroup()
	if w.db != nil {
		w.db.Close()
	}
}

// DeleteDb deletes the wallet database.
func (w *Wallet) DeleteDb() {
	w.db.DeleteDb()
}

func newWallet(config Config, db Db) (*Wallet, error) {
	return &Wallet{
		db:               db,
		config:           config,
		dustCollectorJob: cron.New(),
		dcWg:             newDcWaitGroup(),
		alphaBillClient: abclient.New(abclient.AlphabillClientConfig{
			Uri:              config.AlphabillClientConfig.Uri,
			RequestTimeoutMs: config.AlphabillClientConfig.RequestTimeoutMs,
		}),
		syncFlag: &syncFlagWrapper{},
	}, nil
}

// syncLedger downloads and processes blocks
func (w *Wallet) syncLedger(syncForever bool) error {
	log.Info("staring synchronization process")
	if w.syncFlag.isSynchronizing() {
		log.Warning("wallet is already synchronizing")
		return nil
	}
	w.syncFlag.setSynchronizing(true)
	defer w.syncFlag.setSynchronizing(false)

	blockNo, err := w.db.GetBlockHeight()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	ch := make(chan *alphabill.Block, prefetchBlockCount)
	go w.fetchBlocks(syncForever, blockNo, ch, &wg)
	go func() {
		err = w.processBlocks(ch)
		if err != nil {
			log.Error("error processing block: ", err)
			// in case of error in block processing gorouting we need to either
			// 1) retry the block or 2) signal block receiver goroutine to stop fetching blocks,
			// we do 2) by shutting down the entire wallet.
			w.Shutdown()
		} else {
			log.Info("block processor channel closed")
		}
		wg.Done()
	}()
	wg.Wait()
	log.Info("alphabill sync finished")
	return nil
}

func (w *Wallet) fetchBlocks(syncForever bool, blockNo uint64, ch chan<- *alphabill.Block, wg *sync.WaitGroup) {
	if syncForever {
		for {
			maxBlockNo, err := w.alphaBillClient.GetMaxBlockNo()
			if err != nil {
				log.Error("error fetching max block no: ", err)
				break
			}
			if blockNo == maxBlockNo {
				time.Sleep(sleepTimeAtMaxBlockHeightMs * time.Millisecond)
				continue
			}
			blockNo = blockNo + 1
			block, err := w.alphaBillClient.GetBlock(blockNo)
			if err != nil {
				log.Error("error receiving block: ", err)
				break
			}
			ch <- block
		}
	} else {
		maxBlockNo, err := w.alphaBillClient.GetMaxBlockNo()
		if err != nil {
			log.Error("error fetching max block no: ", err)
		} else {
			for blockNo < maxBlockNo {
				blockNo = blockNo + 1
				block, err := w.alphaBillClient.GetBlock(blockNo)
				if err != nil {
					log.Error("error receiving block: ", err)
					break
				}
				ch <- block
			}
		}
	}
	log.Info("closing block receiver channel, last received block: " + strconv.FormatUint(blockNo, 10))
	close(ch)
	wg.Done()
}

func (w *Wallet) processBlocks(ch <-chan *alphabill.Block) error {
	for b := range ch {
		log.Info("Processing block: " + strconv.FormatUint(b.BlockNo, 10))
		err := w.processBlock(b)
		if err != nil {
			return err
		}
		err = w.postProcessBlock(b)
		if err != nil {
			return err
		}
	}
	return nil
}

// TODO add walletdb memory layer: https://guardtime.atlassian.net/browse/AB-100
func (w *Wallet) processBlock(b *alphabill.Block) error {
	return w.db.WithTransaction(func() error {
		blockHeight, err := w.db.GetBlockHeight()
		if err != nil {
			return err
		}
		err = validateBlockHeight(b, blockHeight)
		if err != nil {
			return err
		}
		for _, txPb := range b.Transactions {
			err = w.collectBills(txPb, blockHeight)
			if err != nil {
				return err
			}
		}
		err = w.deleteExpiredDcBills(b.BlockNo)
		if err != nil {
			return err
		}
		err = w.db.SetBlockHeight(b.BlockNo)
		if err != nil {
			return err
		}
		err = w.trySwap()
		if err != nil {
			return err
		}
		return nil
	})
}

// postProcessBlock called after successful commit on block processing
func (w *Wallet) postProcessBlock(b *alphabill.Block) error {
	return w.dcWg.DecrementSwaps(b.BlockNo, w.db)
}

func (w *Wallet) trySwap() error {
	blockHeight, err := w.db.GetBlockHeight()
	if err != nil {
		return err
	}
	maxBlockNo, err := w.alphaBillClient.GetMaxBlockNo()
	if err != nil {
		return err
	}
	bills, err := w.db.GetBills()
	if err != nil {
		return err
	}
	dcBillGroups := groupDcBills(bills)
	for nonce, billGroup := range dcBillGroups {
		nonce32 := nonce.Bytes32()
		dcMeta, err := w.db.GetDcMetadata(nonce32[:])
		if err != nil {
			return err
		}
		if dcMeta != nil && dcMeta.isSwapRequired(blockHeight, billGroup.valueSum) {
			timeout := maxBlockNo + swapTimeoutBlockCount
			err := w.swapDcBills(billGroup.dcBills, billGroup.dcNonce, timeout)
			if err != nil {
				return err
			}
			w.dcWg.UpdateTimeout(billGroup.dcNonce, timeout)
		}
	}

	// delete expired metadata
	nonceMetadataMap, err := w.db.GetDcMetadataMap()
	if err != nil {
		return err
	}
	for nonce, m := range nonceMetadataMap {
		if m.timeoutReached(blockHeight) {
			nonce32 := nonce.Bytes32()
			err := w.db.SetDcMetadata(nonce32[:], nil)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Wallet) swapDcBills(dcBills []*bill, dcNonce []byte, timeout uint64) error {
	k, err := w.db.GetAccountKey()
	if err != nil {
		return err
	}
	tx, err := createSwapTx(k, dcBills, dcNonce, timeout)
	if err != nil {
		return err
	}
	log.Info("sending swap tx")
	res, err := w.alphaBillClient.SendTransaction(tx)
	if err != nil {
		return err
	}
	if !res.Ok {
		return errors.New("swap tx returned error code: " + res.Message)
	}
	return w.db.SetDcMetadata(dcNonce, &dcMetadata{SwapTimeout: timeout})
}

func (w *Wallet) collectBills(txPb *transaction.Transaction, blockHeight uint64) error {
	gtx, err := transaction.NewMoneyTx(txPb)
	if err != nil {
		return err
	}
	stx := gtx.(txsystem.GenericTransaction)

	switch tx := stx.(type) {
	case money.Transfer:
		isOwner, err := w.isOwner(tx.NewBearer())
		if err != nil {
			return err
		}
		if isOwner {
			err = w.db.SetBill(&bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
		} else {
			err := w.db.RemoveBill(tx.UnitID())
			if err != nil {
				return err
			}
		}
	case money.TransferDC:
		isOwner, err := w.isOwner(tx.TargetBearer())
		if err != nil {
			return err
		}
		if isOwner {
			err = w.db.SetBill(&bill{
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
			err := w.db.RemoveBill(tx.UnitID())
			if err != nil {
				return err
			}
		}
	case money.Split:
		// split tx contains two bills: existing bill and new bill
		// if any of these bills belong to wallet then we have to
		// 1) update the existing bill and
		// 2) add the new bill
		containsBill, err := w.db.ContainsBill(tx.UnitID())
		if err != nil {
			return err
		}
		if containsBill {
			err := w.db.SetBill(&bill{
				Id:     tx.UnitID(),
				Value:  tx.RemainingValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
		}
		isOwner, err := w.isOwner(tx.TargetBearer())
		if err != nil {
			return err
		}
		if isOwner {
			err := w.db.SetBill(&bill{
				Id:     util.SameShardId(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256)),
				Value:  tx.Amount(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
		}
	case money.Swap:
		isOwner, err := w.isOwner(tx.OwnerCondition())
		if err != nil {
			return err
		}
		if isOwner {
			err = w.db.SetBill(&bill{
				Id:     tx.UnitID(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}

			// clear dc metadata
			err = w.db.SetDcMetadata(txPb.UnitId, nil)
			if err != nil {
				return err
			}

			for _, dustTransfer := range tx.DCTransfers() {
				err := w.db.RemoveBill(dustTransfer.UnitID())
				if err != nil {
					return err
				}
			}
		} else {
			err := w.db.RemoveBill(tx.UnitID())
			if err != nil {
				return err
			}
		}
	default:
		panic(fmt.Sprintf("received unknown transaction: %s", tx))
	}
	return nil
}

// isOwner checks if given p2pkh bearer predicate contains Wallet's pubKey hash
func (w *Wallet) isOwner(bp []byte) (bool, error) {
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
		k, err := w.db.GetAccountKey()
		if err != nil {
			return false, err
		}
		return bytes.Equal(bp[6:38], k.PubKeyHashSha256), nil
	} else if hashAlgo == 0x02 {
		k, err := w.db.GetAccountKey()
		if err != nil {
			return false, err
		}
		return bytes.Equal(bp[6:70], k.PubKeyHashSha512), nil
	}
	return false, nil
}

// collectDust sends dust transfer for every bill in wallet and records metadata.
// Once the dust transfers get confirmed on the ledger then swap transfer is broadcast and metadata cleared.
// If blocking is true then the function blocks until swap has been completed or timed out,
// if blocking is false then the function returns after sending the dc transfers.
func (w *Wallet) collectDust(blocking bool) error {
	err := w.db.WithTransaction(func() error {
		blockHeight, err := w.db.GetBlockHeight()
		if err != nil {
			return err
		}
		maxBlockNo, err := w.alphaBillClient.GetMaxBlockNo()
		if err != nil {
			return err
		}
		bills, err := w.db.GetBills()
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
					err = w.swapDcBills(v.dcBills, v.dcNonce, swapTimeout)
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
			swapInProgress, err := w.isSwapInProgress()
			if err != nil {
				return err
			}
			if swapInProgress {
				log.Warning("cannot start dust collection while previous collection round is still in progress")
				return nil
			}

			k, err := w.db.GetAccountKey()
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
				res, err := w.alphaBillClient.SendTransaction(tx)
				if err != nil {
					return err
				}
				if !res.Ok {
					return errors.New("dust transfer returned error code: " + res.Message)
				}
			}
			expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: dcNonce, timeout: dcTimeout})
			err = w.db.SetDcMetadata(dcNonce, &dcMetadata{
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
		log.Info("blocking collect dust waiting")
		w.dcWg.wg.Wait()
		log.Info("blocking collect dust finished")
	}
	return nil
}

// isSwapInProgress returns true if there's a running dc process managed by the wallet
func (w *Wallet) isSwapInProgress() (bool, error) {
	blockHeight, err := w.db.GetBlockHeight()
	if err != nil {
		return false, err
	}
	dcMetadataMap, err := w.db.GetDcMetadataMap()
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

func (w *Wallet) deleteExpiredDcBills(blockHeight uint64) error {
	bills, err := w.db.GetBills()
	if err != nil {
		return err
	}
	for _, b := range bills {
		if b.isExpired(blockHeight) {
			err = w.db.RemoveBill(b.Id)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func createWallet(mnemonic string, config Config) (*Wallet, error) {
	db, err := getDb(config, true)
	if err != nil {
		return nil, err
	}

	err = db.SetEncrypted(config.WalletPass != "")
	if err != nil {
		return nil, err
	}

	err = generateKeys(mnemonic, db)
	if err != nil {
		return nil, err
	}

	return newWallet(config, db)
}

func generateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(mnemonicEntropyBitSize)
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

func generateKeys(mnemonic string, db Db) error {
	if !bip39.IsMnemonicValid(mnemonic) {
		return errors.New("mnemonic is invalid")
	}
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return err
	}

	// https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki
	// m / purpose' / coin_type' / account' / change / address_index
	// m - master key
	// 44' - cryptocurrencies
	// 634' - coin type, randomly chosen number from https://github.com/satoshilabs/slips/blob/master/slip-0044.md
	// 0' - account number (currently use only one account)
	// 0 - change address 0 or 1; 0 = externally used address, 1 = internal address, currently always 0
	// 0 - address index
	// we currently have an ethereum like account based model meaning 1 account = 1 address and no plans to support multiple accounts at this time,
	// so we use wallet's "HD" part only for generating single key from seed
	derivationPath := "m/44'/634'/0'/0/0"

	// only HDPrivateKeyID is used from chaincfg.MainNetParams,
	// it is used as version flag in extended key, which in turn is used to identify the extended key's type.
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return err
	}
	k, err := newAccountKey(masterKey, derivationPath)
	if err != nil {
		return err
	}
	err = db.SetAccountKey(k)
	if err != nil {
		return err
	}
	err = db.SetMasterKey(masterKey.String())
	if err != nil {
		return err
	}
	err = db.SetMnemonic(mnemonic)
	if err != nil {
		return err
	}
	return nil
}

func getDb(config Config, create bool) (Db, error) {
	var db Db
	var err error
	if config.Db == nil {
		if create {
			db, err = createNewDb(config)
		} else {
			db, err = OpenDb(config)
		}
		if err != nil {
			return nil, err
		}
	} else {
		db = config.Db
	}
	return db, nil
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

func validateBlockHeight(b *alphabill.Block, blockHeight uint64) error {
	// verify that we are processing blocks sequentially
	// TODO verify last prev block hash?
	// TODO will genesis block be height 0 or 1: https://guardtime.atlassian.net/browse/AB-101
	if b.BlockNo-blockHeight != 1 {
		return errors.New(fmt.Sprintf("Invalid block height. Received height %d current wallet height %d", b.BlockNo, blockHeight))
	}
	return nil
}
