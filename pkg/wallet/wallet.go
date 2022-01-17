package wallet

import (
	"alphabill-wallet-sdk/internal/abclient"
	"alphabill-wallet-sdk/internal/alphabill/script"
	"alphabill-wallet-sdk/internal/alphabill/txsystem"
	abcrypto "alphabill-wallet-sdk/internal/crypto"
	"alphabill-wallet-sdk/internal/crypto/hash"
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"alphabill-wallet-sdk/internal/util"
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
	"sync"
)

const prefetchBlockCount = 10
const dcTimeoutBlockCount = 10
const swapTimeoutBlockCount = 60
const mnemonicEntropyBitSize = 128

type Wallet struct {
	config           *Config
	db               Db
	alphaBillClient  abclient.ABClient
	dustCollectorJob *cron.Cron
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
// the value returned is the smallest denomination of alphabills (10^15)
func (w *Wallet) GetBalance() (uint64, error) {
	return w.db.GetBalance()
}

// Send creates, signs and broadcasts a transaction of the given amount (in the smallest denomination of alphabills)
// to the given public key
func (w *Wallet) Send(pubKey []byte, amount uint64) error {
	if len(pubKey) != 33 {
		return errors.New("invalid public key, must be 33 bytes in length")
	}
	if w.alphaBillClient == nil {
		return errors.New("alphabill client not initialized, need to sync with alphabill node before attempting to send transactions")
	}
	if w.alphaBillClient.IsShutdown() {
		return errors.New("alphabill client connection is shut down, resync with alphabill node before attempting to send transactions")
	}

	balance, err := w.GetBalance()
	if err != nil {
		return err
	}
	if amount > balance {
		return errors.New("cannot send more than existing balance")
	}

	b, err := w.db.GetBillWithMinValue(amount)
	if err != nil {
		return err
	}

	// todo if bill equals exactly the amount to send then do transfer order instead of split?
	// what would node do if it received split with exact same amount as bill?

	txRpc, err := w.createSplitTx(amount, pubKey, b)
	if err != nil {
		return err
	}
	res, err := w.alphaBillClient.SendTransaction(txRpc)
	if err != nil {
		return err
	}

	if !res.Ok {
		return errors.New("payment returned error code: " + res.Message)
	}
	return nil
}

// Sync synchronises wallet with given alphabill node, blocks forever or until alphabill connection is terminated
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

func (w *Wallet) syncWithAlphaBill() {
	height, err := w.db.GetBlockHeight()
	if err != nil {
		return
	}

	var wg sync.WaitGroup // used to wait for goroutines to close
	wg.Add(2)
	ch := make(chan *alphabill.Block, prefetchBlockCount)
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

func (w *Wallet) createSplitTx(amount uint64, pubKey []byte, bill *bill) (*transaction.Transaction, error) {
	txSig, err := w.signBytes(bill.TxHash) // TODO sign correct data
	if err != nil {
		return nil, err
	}
	k, err := w.db.GetAccountKey()
	if err != nil {
		return nil, err
	}
	ownerProof := script.PredicateArgumentPayToPublicKeyHashDefault(txSig, k.PubKeyHashSha256)

	billId := bill.Id.Bytes32()
	tx := &transaction.Transaction{
		UnitId:                billId[:],
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

func (w *Wallet) createDustTx(bill *bill, nonce *uint256.Int, timeout uint64) (*transaction.Transaction, error) {
	txSig, err := w.signBytes(bill.TxHash) // TODO sign correct data
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

	nonceBytes := nonce.Bytes()
	err = anypb.MarshalFrom(tx.TransactionAttributes, &transaction.TransferDC{
		TargetValue:  bill.Value,
		TargetBearer: script.PredicatePayToPublicKeyHashDefault(k.PubKeyHashSha256),
		Backlink:     bill.TxHash,
		Nonce:        nonceBytes[:],
	}, proto.MarshalOptions{})

	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (w *Wallet) createSwapTx(dcBills []*bill, timeout uint64) (*transaction.Transaction, error) {
	if len(dcBills) == 0 {
		return nil, errors.New("cannot create swap transaction as no dust bills exist")
	}

	var dustTxs []*dustTx
	for _, b := range dcBills {
		dustTxs = append(dustTxs, &dustTx{
			billId:    b.getId(),
			billValue: b.Value,
			tx:        b.DcTx,
			proof:     nil, // TODO get DC proof somewhere
		})
	}

	txSig, err := w.signBytes([]byte{}) // TODO sign correct data
	if err != nil {
		return nil, err
	}

	k, err := w.db.GetAccountKey()
	if err != nil {
		return nil, err
	}

	var billIds [][]byte
	var dustTransferProofs [][]byte
	var dustTransferOrders []*transaction.Transaction
	var billValueSum uint64
	hasher := crypto.SHA256.New()
	for _, dcTx := range dustTxs {
		billIds = append(billIds, dcTx.billId)
		dustTransferOrders = append(dustTransferOrders, dcTx.tx)
		dustTransferProofs = append(dustTransferProofs, dcTx.proof)
		billValueSum += dcTx.billValue
		hasher.Write(dcTx.billId)
	}
	dcBillsHash := hasher.Sum(nil)

	ownerProof := script.PredicateArgumentPayToPublicKeyHashDefault(txSig, k.PubKeyHashSha256)
	unitId := uint256.NewInt(0).SetBytes(dcBillsHash).Bytes32()
	swapTx := &transaction.Transaction{
		UnitId:                unitId[:],
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

func (w *Wallet) initBlockProcessor(ch <-chan *alphabill.Block) error {
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
	// TODO will genesis block be height 0 or 1?
	if b.BlockNo-height != 1 {
		return errors.New(fmt.Sprintf("Invalid block height. Received height %d current wallet height %d", b.BlockNo, height))
	}
	return nil
}

// TODO block processing should be done in a single transaction
// TODO implement memory layer over wallet db so that disk is not touched unless necessary
func (w *Wallet) processBlock(b *alphabill.Block) error {
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
	err = w.trySwap()
	if err != nil {
		return err
	}
	return nil
}

func (w *Wallet) trySwap() error {
	blockHeight, err := w.db.GetBlockHeight()
	if err != nil {
		return err
	}
	dcTimeout, err := w.db.GetDcTimeout()
	if err != nil {
		return err
	}
	swapTimeout, err := w.db.GetSwapTimeout()
	if err != nil {
		return err
	}
	requiredDcSum, err := w.db.GetDcValueSum()
	if err != nil {
		return err
	}
	if requiredDcSum > 0 {
		bills, err := w.db.GetBills()
		if err != nil {
			return err
		}
		var dcSum uint64
		var dcBills []*bill
		for _, b := range bills {
			if b.IsDcBill {
				dcSum += b.Value
				dcBills = append(dcBills, b)
			}
		}
		if dcSum >= requiredDcSum {
			err := w.swapDcBills(dcBills, blockHeight+swapTimeoutBlockCount)
			if err != nil {
				return err
			}
		}
	}
	if blockHeight == dcTimeout || blockHeight == swapTimeout {
		err = w.db.SetDcTimeout(0)
		if err != nil {
			return err
		}
		err = w.db.SetSwapTimeout(0)
		if err != nil {
			return err
		}
		err = w.db.SetDcValueSum(0)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) swapDcBills(dcBills []*bill, timeout uint64) error {
	tx, err := w.createSwapTx(dcBills, timeout)
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

	err = w.db.SetDcTimeout(0)
	if err != nil {
		return err
	}
	err = w.db.SetDcValueSum(0)
	if err != nil {
		return err
	}
	err = w.db.SetSwapTimeout(timeout)
	if err != nil {
		return err
	}
	return nil
}

func (w *Wallet) collectBills(txPb *transaction.Transaction) error {
	gtx, err := transaction.New(txPb)
	if err != nil {
		return err
	}
	stx := gtx.(txsystem.GenericTransaction)

	switch tx := stx.(type) {
	case txsystem.Transfer:
		if w.isOwner(tx.NewBearer()) {
			err = w.db.SetBill(&bill{
				Id:       tx.UnitId(),
				Value:    tx.TargetValue(),
				TxHash:   tx.Hash(crypto.SHA256),
				IsDcBill: false,
				DcTx:     nil,
			})
		} else {
			err := w.db.RemoveBill(tx.UnitId())
			if err != nil {
				return err
			}
		}
	case txsystem.TransferDC:
		if w.isOwner(tx.TargetBearer()) {
			err = w.db.SetBill(&bill{
				Id:       tx.UnitId(),
				Value:    tx.TargetValue(),
				TxHash:   tx.Hash(crypto.SHA256),
				IsDcBill: true,
				DcTx:     txPb,
				DcNonce:  tx.Nonce(),
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
		if w.isOwner(tx.TargetBearer()) {
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
		if w.isOwner(tx.OwnerCondition()) {
			err = w.db.SetBill(&bill{
				Id:     tx.UnitId(),
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
			// TODO once DC bill gets deleted how is it reflected in the ledger?
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
func (w *Wallet) isOwner(bp []byte) bool {
	// p2pkh predicate: [0x53, 0x76, 0xa8, 0x01, 0x4f, 0x01, <32 bytes>, 0x87, 0x69, 0xac, 0x01]
	// p2pkh predicate: [Dup, Hash <SHA256>, PushHash <SHA256> <32 bytes>, Equal, Verify, CheckSig <secp256k1>]

	// p2pkh owner predicate must be 10 + (32 or 64) (SHA256 or SHA512) bytes long
	if len(bp) != 42 && len(bp) != 74 {
		return false
	}
	// 5th byte is PushHash 0x4f
	if bp[4] != 0x4f {
		return false
	}
	// 6th byte is HashAlgo 0x01 or 0x02 for SHA256 and SHA512 respectively
	hashAlgo := bp[5]
	if hashAlgo == 0x01 {
		k, err := w.db.GetAccountKey()
		if err != nil {
			return false // ignore error
		}
		return bytes.Equal(bp[6:38], k.PubKeyHashSha256)
	} else if hashAlgo == 0x02 {
		k, err := w.db.GetAccountKey()
		if err != nil {
			return false // ignore error
		}
		return bytes.Equal(bp[6:70], k.PubKeyHashSha512)
	}
	return false
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
	blockHeight, err := w.db.GetBlockHeight()
	if err != nil {
		return err
	}
	dcTimeout, err := w.db.GetDcTimeout()
	if err != nil {
		return err
	}
	swapTimeout, err := w.db.GetSwapTimeout()
	if err != nil {
		return err
	}
	if blockHeight < dcTimeout || blockHeight < swapTimeout {
		log.Info("cannot start dust collection while previous collection round is still in progress")
		return nil
	}

	bills, err := w.db.GetBills()
	if err != nil {
		return err
	}
	if len(bills) < 2 {
		return nil
	}

	dcNonce, err := w.db.GetDcNonce()
	if err != nil {
		return err
	}
	if dcNonce == nil {
		dcNonce, err = util.RandomUint256()
		if err != nil {
			return err
		}
	}

	err = w.db.SetDcTimeout(blockHeight + dcTimeoutBlockCount)
	if err != nil {
		return err
	}

	var dcValueSum uint64
	for _, b := range bills {
		dcValueSum += b.Value
		if b.IsDcBill {
			continue // no need to send already confirmed dust transactions again
		}

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
	return w.db.SetDcValueSum(dcValueSum)
}

func (w *Wallet) startDustCollectorJob() (cron.EntryID, error) {
	return w.dustCollectorJob.AddFunc("@hourly", func() {
		err := w.collectDust()
		if err != nil {
			log.Error("error in dust collector job: ", err)
		}
	})
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

	// TODO what is HDPrivateKeyID in MainNetParams that is used for key generation
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

// dustTx helper struct for building swap transaction
type dustTx struct {
	billId    []byte
	billValue uint64
	tx        *transaction.Transaction // dustTx
	proof     []byte
}
