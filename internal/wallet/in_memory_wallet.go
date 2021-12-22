package wallet

import (
	"alphabill-wallet-sdk/internal/abclient"
	"alphabill-wallet-sdk/internal/alphabill/domain"
	"alphabill-wallet-sdk/internal/alphabill/rpc"
	"alphabill-wallet-sdk/internal/alphabill/script"
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
)

type InMemoryWallet struct {
	key *key

	// block db
	billContainer *billContainer
	blockHeight   uint64

	// network
	alphaBillClient *abclient.AlphaBillClient
	txMapper        rpc.TransactionMapper
}

func NewInMemoryWallet() (*InMemoryWallet, error) {
	key, err := NewKey()
	if err != nil {
		return nil, err
	}
	w := &InMemoryWallet{
		key:           key,
		billContainer: NewBillContainer(),
		txMapper:      rpc.NewDefaultTxMapper(),
	}
	return w, nil
}

func (w *InMemoryWallet) GetBalance() uint64 {
	return w.billContainer.getBalance()
}

func (w *InMemoryWallet) Send(pubKey []byte, amount uint64) error {
	if amount > w.GetBalance() {
		return errors.New("cannot send more than existing balance")
	}

	bill, exists := w.billContainer.getBillWithMinValue(amount)
	if !exists {
		return errors.New("no spendable bill found")
	}

	// todo if bill equals exactly the amount to send then do transfer order instead of split?
	// what would node do if it received split with exact same amount as bill?

	txRpc, err := w.createSplitTx(amount, pubKey, &bill)
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

func (w *InMemoryWallet) createSplitTx(amount uint64, pubKey []byte, bill *bill) (*transaction.Transaction, error) {
	txSig, err := w.signBytes(bill.txHash) // TODO sign correct data
	if err != nil {
		return nil, err
	}
	billOwnerProof := script.PredicateArgumentPayToPublicKeyHashDefault(txSig, w.key.pubKeyHashSha256)

	tx := &transaction.Transaction{
		UnitId:                bill.id.Bytes(),
		TransactionAttributes: new(anypb.Any),
		Timeout:               1000,
		OwnerProof:            billOwnerProof,
	}

	err = anypb.MarshalFrom(tx.TransactionAttributes, &transaction.BillSplit{
		Amount:         bill.value,
		TargetBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		RemainingValue: bill.value - amount,
		Backlink:       bill.txHash,
	}, proto.MarshalOptions{})

	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (w *InMemoryWallet) Sync(conf *config.AlphaBillClientConfig) error {
	abClient, err := abclient.New(conf)
	if err != nil {
		return err
	}
	w.alphaBillClient = abClient

	ch := make(chan *alphabill.Block, 10) // randomly chosen number, prefetches up to 10 blocks from AB node

	go func() {
		err := w.alphaBillClient.InitBlockReceiver(w.blockHeight, ch)
		if err != nil {
			log.Printf("error receiving block %s", err) // TODO how to log in embedded SDK?
			abClient.Shutdown()
		}
	}()
	go func() {
		err := w.initBlockProcessor(ch)
		if err != nil {
			log.Printf("error processing block %s", err) // TODO how to log in embedded SDK?
			abClient.Shutdown()
		}
	}()

	return nil
}

func (w *InMemoryWallet) Shutdown() {
	if w.alphaBillClient != nil {
		w.alphaBillClient.Shutdown()
	}
}

func (w *InMemoryWallet) initBlockProcessor(ch chan *alphabill.Block) error {
	for {
		b, ok := <-ch
		if !ok {
			return nil
		}
		err := w.processBlock(b)
		if err != nil {
			return err
		}
	}
}

func (w *InMemoryWallet) verifyBlockHeight(b *alphabill.Block) bool {
	// verify that we are processing blocks sequentially
	// it may be possible to improve block processing speed by parallel processing
	// TODO will genesis block be height 0 or 1?
	return b.BlockNo-w.blockHeight == 1
}

func (w *InMemoryWallet) processBlock(b *alphabill.Block) error {
	if !w.verifyBlockHeight(b) {
		return errors.New(fmt.Sprintf("Invalid block height. Received height %d current wallet height %d", b.BlockNo, w.blockHeight))
	}
	for _, txPb := range b.Transactions {
		tx, err := w.txMapper.MapPbToDomain(txPb)
		if err != nil {
			return err
		}
		w.collectBills(tx)
	}
	w.blockHeight = b.BlockNo
	return nil
}

func (w *InMemoryWallet) collectBills(tx *domain.Transaction) {
	txSplit, isSplitTx := tx.TransactionAttributes.(*domain.BillSplit)
	if isSplitTx {
		if w.billContainer.containsBill(tx.UnitId) {
			w.billContainer.addBill(bill{
				id:     tx.UnitId,
				value:  txSplit.RemainingValue,
				txHash: tx.Hash(),
			})
		}
		if w.isOwner(txSplit.TargetBearer) {
			w.billContainer.addBill(bill{
				id:     uint256.NewInt(0).SetBytes(tx.Hash()), // TODO generate id properly
				value:  txSplit.Amount,
				txHash: tx.Hash(),
			})
		}
	} else {
		if w.isOwner(tx.TransactionAttributes.OwnerCondition()) {
			w.billContainer.addBill(bill{
				id:     tx.UnitId,
				value:  tx.TransactionAttributes.Value(),
				txHash: tx.Hash(),
			})
		} else {
			w.billContainer.removeBillIfExists(tx.UnitId)
		}
	}
}

// isOwner checks if given p2pkh bearer predicate contains wallet's pubKey hash
func (w *InMemoryWallet) isOwner(bp []byte) bool {
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
		return bytes.Equal(bp[6:38], w.key.pubKeyHashSha256)
	} else if hashAlgo == 0x02 {
		return bytes.Equal(bp[6:70], w.key.pubKeyHashSha512)
	}
	return false
}

func (w *InMemoryWallet) signBytes(b []byte) ([]byte, error) {
	return w.key.signer.SignBytes(b)
}
