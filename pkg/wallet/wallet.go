package wallet

import (
	"alphabill-wallet-sdk/internal/abclient"
	"alphabill-wallet-sdk/internal/alphabill/script"
	"alphabill-wallet-sdk/internal/alphabill/txsystem"
	abcrypto "alphabill-wallet-sdk/internal/crypto"
	"alphabill-wallet-sdk/internal/crypto/hash"
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"alphabill-wallet-sdk/pkg/log"
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/holiman/uint256"
	"github.com/robfig/cron/v3"
	"github.com/tyler-smith/go-bip39"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"sort"
	"sync"
)

const (
	prefetchBlockCount       = 10
	dcTimeoutBlockCount      = 10
	swapTimeoutBlockCount    = 60
	isSyncedCutoffBlockCount = 10
	mnemonicEntropyBitSize   = 128
)

var (
	errInvalidPubKey          = errors.New("invalid public key, public key must be in compressed secp256k1 format")
	errABClientNotInitialized = errors.New("alphabill client not initialized, need to sync with alphabill node before attempting to send transactions")
	errABClientNotConnected   = errors.New("alphabill client connection is shut down, resync with alphabill node before attempting to send transactions")
	errSwapInProgress         = errors.New("swap is in progress, please wait for swap process to be completed before attempting to send transactions")
	errInvalidBalance         = errors.New("cannot send more than existing balance")
)

type Wallet struct {
	config           *Config
	db               Db
	alphaBillClient  abclient.ABClient
	dustCollectorJob *cron.Cron
}

// dcBillGroup helper struct for grouped dc bills and their aggregate data
type dcBillGroup struct {
	dcBills   []*bill
	valueSum  uint64
	dcNonce   []byte
	dcTimeout uint64
}

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func CreateNewWallet(config *Config) (*Wallet, error) {
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
func CreateWalletFromSeed(mnemonic string, config *Config) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}
	return createWallet(mnemonic, config)
}

// LoadExistingWallet loads an existing wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func LoadExistingWallet(config *Config) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}

	db, err := getDb(config, false)
	if err != nil {
		return nil, err
	}

	return &Wallet{
		db:               db,
		config:           config,
		dustCollectorJob: cron.New(),
	}, nil
}

// GetBalance returns sum value of all bills currently owned by the wallet
// the value returned is the smallest denomination of alphabills
func (w *Wallet) GetBalance() (uint64, error) {
	return w.db.GetBalance()
}

// Send creates, signs and broadcasts a transaction of the given amount (in the smallest denomination of alphabills)
// to the given public key, the public key must be in compressed secp256k1 format
func (w *Wallet) Send(pubKey []byte, amount uint64) error {
	if len(pubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
		return errInvalidPubKey
	}
	if w.alphaBillClient == nil {
		return errABClientNotInitialized
	}
	if w.alphaBillClient.IsShutdown() {
		return errABClientNotConnected
	}

	swapInProgress, err := w.isSwapInProgress()
	if err != nil {
		return err
	}
	if swapInProgress {
		return errSwapInProgress
	}

	balance, err := w.GetBalance()
	if err != nil {
		return err
	}
	if amount > balance {
		return errInvalidBalance
	}

	b, err := w.db.GetBillWithMinValue(amount)
	if err != nil {
		return err
	}

	tx, err := w.createTransaction(pubKey, amount, b)
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
// it blocks forever or until alphabill connection is terminated
func (w *Wallet) Sync() error {
	abClient, err := abclient.New(&abclient.AlphaBillClientConfig{Uri: w.config.AlphaBillClientConfig.Uri})
	if err != nil {
		return err
	}
	_, err = w.startDustCollectorJob()
	if err != nil {
		return err
	}
	w.alphaBillClient = abClient
	w.syncWithAlphaBill()
	return nil
}

// Shutdown terminates connection to alphabill node, closes wallet db and any background goroutines
func (w *Wallet) Shutdown() {
	if w.alphaBillClient != nil {
		w.alphaBillClient.Shutdown()
	}
	if w.db != nil {
		w.db.Close()
	}
	if w.dustCollectorJob != nil {
		w.dustCollectorJob.Stop()
	}
}

// DeleteDb deletes the wallet database
func (w *Wallet) DeleteDb() {
	w.db.DeleteDb()
}

func (w *Wallet) createTransaction(pubKey []byte, amount uint64, b *bill) (*transaction.Transaction, error) {
	var tx *transaction.Transaction
	var err error
	if b.Value == amount {
		tx, err = w.createTransferTx(pubKey, b)
	} else {
		tx, err = w.createSplitTx(amount, pubKey, b)
	}
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (w *Wallet) syncWithAlphaBill(terminateAtMaxHeight bool) {
	height, err := w.db.GetBlockHeight()
	if err != nil {
		return
	}

	var wg sync.WaitGroup // used to wait for goroutines to close
	wg.Add(2)
	ch := make(chan *alphabill.GetBlocksResponse, prefetchBlockCount)
	go func() {
		err = w.alphaBillClient.InitBlockReceiver(height, ch)
		if err != nil {
			log.Error("error receiving block: ", err)
		}
		log.Info("closing block receiver channel")
		close(ch)
		wg.Done()
	}()
	go func() {
		err = w.initBlockProcessor(ch)
		if err != nil {
			log.Error("error processing block: ", err)
		} else {
			log.Info("block processor channel closed")
		}
		w.alphaBillClient.Shutdown()
		wg.Done()
	}()
	wg.Wait()
	log.Info("alphabill sync finished")
}

func (w *Wallet) createTransferTx(pubKey []byte, bill *bill) (*transaction.Transaction, error) {
	txSig, err := w.signBytes(bill.TxHash) // TODO sign correct data: https://guardtime.atlassian.net/browse/AB-102
	if err != nil {
		return nil, err
	}
	k, err := w.db.GetAccountKey()
	if err != nil {
		return nil, err
	}
	ownerProof := script.PredicateArgumentPayToPublicKeyHashDefault(txSig, k.PubKeyHashSha256)

	tx := &transaction.Transaction{
		UnitId:                bill.getId(),
		TransactionAttributes: new(anypb.Any),
		Timeout:               1000,
		OwnerProof:            ownerProof,
	}

	err = anypb.MarshalFrom(tx.TransactionAttributes, &transaction.BillTransfer{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: bill.Value,
		Backlink:    bill.TxHash,
	}, proto.MarshalOptions{})

	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (w *Wallet) createSplitTx(amount uint64, pubKey []byte, bill *bill) (*transaction.Transaction, error) {
	txSig, err := w.signBytes(bill.TxHash) // TODO sign correct data: https://guardtime.atlassian.net/browse/AB-102
	if err != nil {
		return nil, err
	}
	k, err := w.db.GetAccountKey()
	if err != nil {
		return nil, err
	}
	ownerProof := script.PredicateArgumentPayToPublicKeyHashDefault(txSig, k.PubKeyHashSha256)

	tx := &transaction.Transaction{
		UnitId:                bill.getId(),
		TransactionAttributes: new(anypb.Any),
		Timeout:               1000,
		OwnerProof:            ownerProof,
	}

	err = anypb.MarshalFrom(tx.TransactionAttributes, &transaction.BillSplit{
		Amount:         bill.Value,
		TargetBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		RemainingValue: bill.Value - amount,
		Backlink:       bill.TxHash,
	}, proto.MarshalOptions{})

	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (w *Wallet) createDustTx(bill *bill, nonce []byte, timeout uint64) (*transaction.Transaction, error) {
	txSig, err := w.signBytes(bill.TxHash) // TODO sign correct data: https://guardtime.atlassian.net/browse/AB-102
	if err != nil {
		return nil, err
	}
	k, err := w.db.GetAccountKey()
	if err != nil {
		return nil, err
	}
	ownerProof := script.PredicateArgumentPayToPublicKeyHashDefault(txSig, k.PubKeyHashSha256)

	tx := &transaction.Transaction{
		UnitId:                bill.getId(),
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}

	err = anypb.MarshalFrom(tx.TransactionAttributes, &transaction.TransferDC{
		TargetValue:  bill.Value,
		TargetBearer: script.PredicatePayToPublicKeyHashDefault(k.PubKeyHashSha256),
		Backlink:     bill.TxHash,
		Nonce:        nonce,
	}, proto.MarshalOptions{})

	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (w *Wallet) createSwapTx(dcBills []*bill, dcNonce []byte, timeout uint64) (*transaction.Transaction, error) {
	if len(dcBills) == 0 {
		return nil, errors.New("cannot create swap transaction as no dust bills exist")
	}

	txSig, err := w.signBytes([]byte{}) // TODO sign correct data: https://guardtime.atlassian.net/browse/AB-102
	if err != nil {
		return nil, err
	}

	k, err := w.db.GetAccountKey()
	if err != nil {
		return nil, err
	}

	// sort bills by ids in ascending order
	sort.Slice(dcBills, func(i, j int) bool {
		return bytes.Compare(dcBills[i].getId(), dcBills[j].getId()) < 0
	})

	var billIds [][]byte
	var dustTransferProofs [][]byte
	var dustTransferOrders []*transaction.Transaction
	var billValueSum uint64
	for _, b := range dcBills {
		billIds = append(billIds, b.getId())
		dustTransferOrders = append(dustTransferOrders, b.DcTx)
		// TODO add DC proofs: https://guardtime.atlassian.net/browse/AB-99
		dustTransferProofs = append(dustTransferProofs, nil)
		billValueSum += b.Value
	}

	ownerProof := script.PredicateArgumentPayToPublicKeyHashDefault(txSig, k.PubKeyHashSha256)
	swapTx := &transaction.Transaction{
		UnitId:                dcNonce,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}

	err = anypb.MarshalFrom(swapTx.TransactionAttributes, &transaction.Swap{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(k.PubKeyHashSha256),
		BillIdentifiers: billIds,
		DcTransfers:     dustTransferOrders,
		Proofs:          dustTransferProofs,
		TargetValue:     billValueSum,
	}, proto.MarshalOptions{})

	if err != nil {
		return nil, err
	}
	return swapTx, nil
}

func (w *Wallet) initBlockProcessor(ch <-chan *alphabill.GetBlocksResponse) error {
	for b := range ch {
		err := w.processBlock(b)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) verifyBlockHeight(b *alphabill.Block) error {
	// verify that we are processing blocks sequentially
	height, err := w.db.GetBlockHeight()
	if err != nil {
		return err
	}
	// TODO will genesis block be height 0 or 1: https://guardtime.atlassian.net/browse/AB-101
	if b.BlockNo-height != 1 {
		return errors.New(fmt.Sprintf("Invalid block height. Received height %d current wallet height %d", b.BlockNo, height))
	}
	return nil
}

// TODO add walletdb memory layer: https://guardtime.atlassian.net/browse/AB-100
func (w *Wallet) processBlock(blockResponse *alphabill.GetBlocksResponse) error {
	return w.db.WithTransaction(func() error {
		b := blockResponse.Block
		err := w.verifyBlockHeight(b)
		if err != nil {
			return err
		}
		for _, txPb := range b.Transactions {
			err = w.collectBills(txPb)
			if err != nil {
				return err
			}
		}
		err = w.db.SetBlockHeight(b.BlockNo)
		if err != nil {
			return err
		}
		err = w.db.SetMaxBlockHeight(blockResponse.MaxBlockHeight)
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

func (w *Wallet) trySwap() error {
	blockHeight, err := w.db.GetBlockHeight()
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
		if dcMeta.isSwapRequired(blockHeight, billGroup.valueSum) {
			err := w.swapDcBills(billGroup.dcBills, billGroup.dcNonce, blockHeight+swapTimeoutBlockCount)
			if err != nil {
				return err
			}
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
	tx, err := w.createSwapTx(dcBills, dcNonce, timeout)
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

func (w *Wallet) collectBills(txPb *transaction.Transaction) error {
	gtx, err := transaction.New(txPb)
	if err != nil {
		return err
	}
	stx := gtx.(txsystem.GenericTransaction)

	switch tx := stx.(type) {
	case txsystem.Transfer:
		isOwner, err := w.isOwner(tx.NewBearer())
		if err != nil {
			return err
		}
		if isOwner {
			err = w.db.SetBill(&bill{
				Id:     tx.UnitId(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
		} else {
			err := w.db.RemoveBill(tx.UnitId())
			if err != nil {
				return err
			}
		}
	case txsystem.TransferDC:
		isOwner, err := w.isOwner(tx.TargetBearer())
		if err != nil {
			return err
		}
		if isOwner {
			err = w.db.SetBill(&bill{
				Id:        tx.UnitId(),
				Value:     tx.TargetValue(),
				TxHash:    tx.Hash(crypto.SHA256),
				IsDcBill:  true,
				DcTx:      txPb,
				DcTimeout: tx.Timeout(),
				DcNonce:   tx.Nonce(),
			})
		} else {
			err := w.db.RemoveBill(tx.UnitId())
			if err != nil {
				return err
			}
		}
	case txsystem.Split:
		// split tx contains two bills: existing bill and new bill
		// if any of these bills belong to wallet then we have to
		// 1) update the existing bill and
		// 2) add the new bill
		containsBill, err := w.db.ContainsBill(tx.UnitId())
		if err != nil {
			return err
		}
		if containsBill {
			err := w.db.SetBill(&bill{
				Id:     tx.UnitId(),
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
				Id:     txsystem.SameShardId(tx.UnitId(), tx.HashForIdCalculation(crypto.SHA256)),
				Value:  tx.Amount(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
		}
	case txsystem.Swap:
		isOwner, err := w.isOwner(tx.OwnerCondition())
		if err != nil {
			return err
		}
		if isOwner {
			err = w.db.SetBill(&bill{
				Id:     tx.UnitId(),
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

			// TODO once DC bill gets deleted how is it reflected in the ledger? https://guardtime.atlassian.net/browse/AB-101
			// => it's not, wallet must follow blockchain rules, and one of the rules is that after X amount of time
			// any pending dc bill gets removed
			for _, dustTransfer := range tx.DCTransfers() {
				err := w.db.RemoveBill(dustTransfer.UnitId())
				if err != nil {
					return err
				}
			}
		} else {
			err := w.db.RemoveBill(tx.UnitId())
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

func (w *Wallet) signBytes(b []byte) ([]byte, error) {
	k, err := w.db.GetAccountKey()
	if err != nil {
		return nil, err
	}
	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(k.PrivKey)
	if err != nil {
		return nil, err
	}
	return signer.SignBytes(b)
}

// collectDust sends dust transfer for every bill in wallet and records metadata
// once the dust transfers get confirmed on the ledger then swap transfer is broadcast and metadata cleared
func (w *Wallet) collectDust() error {
	return w.db.WithTransaction(func() error {
		isSynced, err := w.isSynced()
		if err != nil {
			return err
		}
		if !isSynced {
			log.Info("cannot start dust collector in an unsynced wallet")
			return nil
		}
		blockHeight, err := w.db.GetBlockHeight()
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
		dcBillGroups := groupDcBills(bills)
		if len(dcBillGroups) > 0 {
			for _, v := range dcBillGroups {
				if blockHeight >= v.dcTimeout {
					err := w.swapDcBills(v.dcBills, v.dcNonce, blockHeight+swapTimeoutBlockCount)
					if err != nil {
						return err
					}
				}
			}
			return nil
		} else {
			swapInProgress, err := w.isSwapInProgress()
			if err != nil {
				return err
			}
			if swapInProgress {
				log.Warning("cannot start dust collection while previous collection round is still in progress")
				return nil
			}

			dcNonce := calculateDcNonce(bills)
			dcTimeout := blockHeight + dcTimeoutBlockCount
			var dcValueSum uint64
			for _, b := range bills {
				dcValueSum += b.Value
				tx, err := w.createDustTx(b, dcNonce, dcTimeout)
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
			err = w.db.SetDcMetadata(dcNonce, &dcMetadata{
				DcValueSum: dcValueSum,
				DcTimeout:  dcTimeout,
			})
			return nil
		}
	})
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
		err := w.collectDust()
		if err != nil {
			log.Error("error in dust collector job: ", err)
		}
	})
}

func (w *Wallet) isSynced() (bool, error) {
	blockHeight, err := w.db.GetBlockHeight()
	if err != nil {
		return false, err
	}
	maxBlockHeight, err := w.db.GetMaxBlockHeight()
	if err != nil {
		return false, err
	}
	return maxBlockHeight-blockHeight < isSyncedCutoffBlockCount, nil
}

func createWallet(mnemonic string, config *Config) (*Wallet, error) {
	db, err := getDb(config, true)
	if err != nil {
		return nil, err
	}

	err = generateKeys(mnemonic, db)
	if err != nil {
		db.DeleteDb()
		return nil, err
	}

	return &Wallet{
		db:               db,
		config:           config,
		dustCollectorJob: cron.New(),
	}, nil
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

func getDb(config *Config, create bool) (Db, error) {
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
