package wallet

import (
	"alphabill-wallet-sdk/internal/abclient"
	"alphabill-wallet-sdk/internal/alphabill/domain"
	"alphabill-wallet-sdk/internal/alphabill/rpc"
	"alphabill-wallet-sdk/internal/alphabill/script"
	abcrypto "alphabill-wallet-sdk/internal/crypto"
	"alphabill-wallet-sdk/internal/crypto/hash"
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"alphabill-wallet-sdk/pkg/log"
	"alphabill-wallet-sdk/pkg/wallet/config"
	"bytes"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/holiman/uint256"
	"github.com/tyler-smith/go-bip39"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"os"
	"sync"
)

const prefetchBlockCount = 10

type Wallet struct {
	config          *config.WalletConfig
	db              Db
	alphaBillClient abclient.ABClient
	txMapper        rpc.TransactionMapper
}

func CreateNewWallet(config *config.WalletConfig) (*Wallet, error) {
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

func generateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

func CreateWalletFromSeed(mnemonic string, config *config.WalletConfig) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}
	return createWallet(mnemonic, config)
}

func createWallet(mnemonic string, config *config.WalletConfig) (*Wallet, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("mnemonicKeyName is invalid")
	}
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return nil, err
	}
	// TODO what is HDPrivateKeyID in MainNetParams that is used for key generation
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}

	db, err := CreateNewDb(config)
	if err != nil {
		return nil, err
	}

	w := &Wallet{
		db:       db,
		config:   config,
		txMapper: rpc.NewDefaultTxMapper(),
	}

	err = db.CreateBuckets()
	if err != nil {
		return w, err
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

	k, err := NewAccountKey(masterKey, derivationPath)
	if err != nil {
		return w, err
	}
	err = db.SetAccountKey(k)
	if err != nil {
		return w, err
	}
	err = db.SetMasterKey(masterKey.String())
	if err != nil {
		return w, err
	}
	err = db.SetMnemonic(mnemonic)
	if err != nil {
		return w, err
	}
	return w, nil
}

func LoadExistingWallet(config *config.WalletConfig) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}

	db, err := OpenDb(config)
	if err != nil {
		return nil, err
	}
	w := &Wallet{
		db:       db,
		txMapper: rpc.NewDefaultTxMapper(),
	}
	return w, nil
}

func (w *Wallet) GetBalance() (uint64, error) {
	return w.db.GetBalance()
}

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

func (w *Wallet) Sync() error {
	abClient, err := abclient.New(w.config.AlphaBillClientConfig)
	if err != nil {
		return err
	}
	w.syncWithAlphaBill(abClient)
	return nil
}

func (w *Wallet) syncWithAlphaBill(abClient abclient.ABClient) {
	w.alphaBillClient = abClient
	height, err := w.db.GetBlockHeight()
	if err != nil {
		return
	}

	var wg sync.WaitGroup // used to wait for channels to close
	wg.Add(2)
	ch := make(chan *alphabill.Block, prefetchBlockCount)
	go func() {
		err = w.alphaBillClient.InitBlockReceiver(height, ch)
		if err != nil {
			log.Error("error receiving block", err)
		}
		close(ch)
		wg.Done()
	}()
	go func() {
		err = w.initBlockProcessor(ch)
		if err != nil {
			log.Error("error processing block", err)
		} else {
			log.Info("block processor channel closed")
		}
		w.alphaBillClient.Shutdown()
		wg.Done()
	}()
	wg.Wait()
	log.Info("alphabill sync finished")
}

func (w *Wallet) Shutdown() {
	if w.alphaBillClient != nil {
		w.alphaBillClient.Shutdown()
	}
	if w.db != nil {
		w.db.Close()
	}
}

func (w *Wallet) DeleteDb() error {
	walletDir, err := w.config.GetWalletDir()
	if err != nil {
		return err
	}
	dbFilePath := walletDir + walletFileName
	return os.Remove(dbFilePath)
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
	// it may be possible to improve block processing speed by parallel processing
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

func (w *Wallet) processBlock(b *alphabill.Block) error {
	err := w.verifyBlockHeight(b)
	if err != nil {
		return err
	}
	for _, txPb := range b.Transactions {
		tx, err := w.txMapper.MapPbToDomain(txPb)
		if err != nil {
			return err
		}
		err = w.collectBills(tx)
		if err != nil {
			return err
		}
	}
	return w.db.SetBlockHeight(b.BlockNo)
}

func (w *Wallet) collectBills(tx *domain.Transaction) error {
	txSplit, isSplitTx := tx.TransactionAttributes.(*domain.BillSplit)
	if isSplitTx {
		containsBill, err := w.db.ContainsBill(tx.UnitId)
		if err != nil {
			return err
		}
		if containsBill {
			err := w.db.SetBill(&bill{
				Id:     tx.UnitId,
				Value:  txSplit.RemainingValue,
				TxHash: tx.Hash(),
			})
			if err != nil {
				return err
			}
		}
		if w.isOwner(txSplit.TargetBearer) {
			err := w.db.SetBill(&bill{
				Id:     uint256.NewInt(0).SetBytes(tx.Hash()), // TODO generate Id properly
				Value:  txSplit.Amount,
				TxHash: tx.Hash(),
			})
			if err != nil {
				return err
			}
		}
	} else {
		if w.isOwner(tx.TransactionAttributes.OwnerCondition()) {
			err := w.db.SetBill(&bill{
				Id:     tx.UnitId,
				Value:  tx.TransactionAttributes.Value(),
				TxHash: tx.Hash(),
			})
			if err != nil {
				return err
			}
		} else {
			err := w.db.RemoveBill(tx.UnitId) // TODO remove in batch or do whole operation collectBills in batch
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// isOwner checks if given p2pkh bearer predicate contains wallet's pubKey hash
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
