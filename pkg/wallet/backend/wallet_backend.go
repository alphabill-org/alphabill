package backend

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/proof"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/holiman/uint256"
)

var (
	alphabillMoneySystemId = []byte{0, 0, 0, 0}
	log                    = logger.CreateForPackage()
)

type (
	WalletBackend struct {
		store         BillStore
		genericWallet *wallet.Wallet
	}

	bill struct {
		Id    *uint256.Int
		Value uint64
	}

	blockProof struct {
		BillId      []byte            `json:"billId"`
		BlockNumber uint64            `json:"blockNumber"`
		BlockProof  *proof.BlockProof `json:"blockProof"`
	}

	pubkey struct {
		pubkey     []byte
		pubkeyHash *wallet.KeyHashes
	}

	BillStore interface {
		GetBlockNumber() (uint64, error)
		SetBlockNumber(blockNumber uint64) error
		GetBills(pubKey []byte) ([]*bill, error)
		AddBill(pubKey []byte, bill *bill) error
		RemoveBill(pubKey []byte, id *uint256.Int) error
		ContainsBill(pubKey []byte, id *uint256.Int) (bool, error)
		GetBlockProof(billId []byte) (*blockProof, error)
		SetBlockProof(proof *blockProof) error
	}
)

func New(pubkeys [][]byte, abclient client.ABClient, store BillStore) *WalletBackend {
	var trackedPubKeys []*pubkey
	for _, pk := range pubkeys {
		trackedPubKeys = append(trackedPubKeys, &pubkey{
			pubkey: pk,
			pubkeyHash: &wallet.KeyHashes{
				Sha256: hash.Sum256(pk),
				Sha512: hash.Sum512(pk),
			},
		})
	}
	bp := newBlockProcessor(store, trackedPubKeys)
	genericWallet := wallet.New().SetBlockProcessor(bp).SetABClient(abclient).Build()
	return &WalletBackend{store: store, genericWallet: genericWallet}
}

// Start starts downloading blocks and indexing bills by their owner's public key
func (w *WalletBackend) Start(ctx context.Context) error {
	blockNumber, err := w.store.GetBlockNumber()
	if err != nil {
		return err
	}
	return w.genericWallet.Sync(ctx, blockNumber)
}

// GetBills returns all bills for given public key
func (w *WalletBackend) GetBills(pubkey []byte) ([]*bill, error) {
	return w.store.GetBills(pubkey)
}

// GetBlockProof returns most recent proof for given unit id
func (w *WalletBackend) GetBlockProof(unitId []byte) (*blockProof, error) {
	return w.store.GetBlockProof(unitId)
}
