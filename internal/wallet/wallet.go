package wallet

import (
	"alphabill-wallet-sdk/internal/abclient"
	"alphabill-wallet-sdk/internal/alphabill/domain"
	"alphabill-wallet-sdk/internal/alphabill/rpc"
	"alphabill-wallet-sdk/internal/alphabill/script"
	"alphabill-wallet-sdk/internal/crypto"
	"alphabill-wallet-sdk/internal/crypto/hash"
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"alphabill-wallet-sdk/pkg/wallet/config"
	"bytes"
	"errors"
	"fmt"
	"github.com/holiman/uint256"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"log"
	"os"
)

type Wallet struct {
	db *Db

	// network
	alphaBillClient abclient.ABClient
	txMapper        rpc.TransactionMapper
}

func CreateNewWallet() (*Wallet, error) {
	walletDir, err := getWalletDir()
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(walletDir, 0700) // -rwx------
	if err != nil {
		return nil, err
	}

	db, err := CreateNewDb(walletDir)
	if err != nil {
		return nil, err
	}

	w := &Wallet{
		db:       db,
		txMapper: rpc.NewDefaultTxMapper(),
	}

	err = db.CreateBuckets()
	if err != nil {
		return w, err
	}

	key, err := NewKey()
	if err != nil {
		return w, err
	}

	err = db.AddKey(key)
	if err != nil {
		return w, err
	}
	return w, nil
}

func LoadExistingWallet() (*Wallet, error) {
	walletDir, err := getWalletDir()
	if err != nil {
		return nil, err
	}
	db, err := OpenDb(walletDir)
	if err != nil {
		return nil, err
	}
	w := &Wallet{
		db:       db,
		txMapper: rpc.NewDefaultTxMapper(),
	}
	return w, nil
}

func getWalletDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	walletDir := homeDir + string(os.PathSeparator) + ".alphabill" + string(os.PathSeparator)
	return walletDir, nil
}

func (w *Wallet) GetBalance() uint64 {
	return w.db.GetBalance()
}

func (w *Wallet) Send(pubKey []byte, amount uint64) error {
	if len(pubKey) != 33 {
		return errors.New("invalid public key, must be 33 bytes in length")
	}
	if amount > w.GetBalance() {
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
	k, err := w.db.GetKey()
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

func (w *Wallet) Sync(conf *config.AlphaBillClientConfig) error {
	abClient, err := abclient.New(conf)
	if err != nil {
		return err
	}
	w.syncWithAlphaBill(abClient)
	return nil
}

func (w *Wallet) syncWithAlphaBill(abClient abclient.ABClient) {
	w.alphaBillClient = abClient
	ch := make(chan *alphabill.Block, 10) // randomly chosen number, prefetches up to 10 blocks from AB node
	go func() {
		err := w.alphaBillClient.InitBlockReceiver(w.db.GetBlockHeight(), ch)
		if err != nil {
			log.Printf("error receiving block %s", err) // TODO how to log in embedded SDK?
			close(ch)
		}
	}()
	go func() {
		err := w.initBlockProcessor(ch)
		if err != nil {
			log.Printf("error processing block %s", err) // TODO how to log in embedded SDK?
		} else {
			log.Printf("block processor channel closed") // TODO how to log in embedded SDK?
		}
		w.alphaBillClient.Shutdown()
	}()
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
	walletDir, _ := getWalletDir()
	dbFilePath := walletDir + "/wallet.db"
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

func (w *Wallet) verifyBlockHeight(b *alphabill.Block) bool {
	// verify that we are processing blocks sequentially
	// it may be possible to improve block processing speed by parallel processing
	// TODO will genesis block be height 0 or 1?
	return b.BlockNo-w.db.GetBlockHeight() == 1
}

func (w *Wallet) processBlock(b *alphabill.Block) error {
	if !w.verifyBlockHeight(b) {
		return errors.New(fmt.Sprintf("Invalid block height. Received height %d current wallet height %d", b.BlockNo, w.db.GetBlockHeight()))
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
		if w.db.ContainsBill(tx.UnitId) {
			err := w.db.AddBill(&bill{
				Id:     *tx.UnitId,
				Value:  txSplit.RemainingValue,
				TxHash: tx.Hash(),
			})
			if err != nil {
				return err
			}
		}
		if w.isOwner(txSplit.TargetBearer) {
			err := w.db.AddBill(&bill{
				Id:     *uint256.NewInt(0).SetBytes(tx.Hash()), // TODO generate Id properly
				Value:  txSplit.Amount,
				TxHash: tx.Hash(),
			})
			if err != nil {
				return err
			}
		}
	} else {
		if w.isOwner(tx.TransactionAttributes.OwnerCondition()) {
			err := w.db.AddBill(&bill{
				Id:     *tx.UnitId,
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
		k, err := w.db.GetKey()
		if err != nil {
			return false // ignore error
		}
		return bytes.Equal(bp[6:38], k.PubKeyHashSha256)
	} else if hashAlgo == 0x02 {
		k, err := w.db.GetKey()
		if err != nil {
			return false // ignore error
		}
		return bytes.Equal(bp[6:70], k.PubKeyHashSha512)
	}
	return false
}

func (w *Wallet) signBytes(b []byte) ([]byte, error) {
	k, err := w.db.GetKey()
	if err != nil {
		return nil, err
	}
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(k.PrivKey)
	if err != nil {
		return nil, err
	}
	return signer.SignBytes(b)
}
