package wallet

import (
	"alphabill-wallet-sdk/internal/alphabill/script"
	"alphabill-wallet-sdk/internal/crypto/hash"
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWalletCanSendTx(t *testing.T) {
	w, err := NewInMemoryWallet()
	require.NoError(t, err)
	w.syncWithAlphaBill(&mockAlphaBillClient{})

	b := bill{
		id:     *uint256.NewInt(0),
		value:  100,
		txHash: hash.Sum256([]byte{0x01}),
	}
	w.billContainer.addBill(b)

	receiverPubKey := make([]byte, 33)
	err = w.Send(receiverPubKey, 50)
	require.NoError(t, err)

	billId := b.id.Bytes32()
	block := &alphabill.Block{
		BlockNo:       1,
		PrevBlockHash: hash.Sum256([]byte{}),
		Transactions: []*transaction.Transaction{
			{
				UnitId:                billId[:],
				TransactionAttributes: createBillSplitTx(hash.Sum256(receiverPubKey), 50, 50),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, w.key.pubKeyHashSha256),
			},
		},
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
	require.NoError(t, err)
	require.EqualValues(t, 50, w.GetBalance())
}

type mockAlphaBillClient struct {
}

func (c *mockAlphaBillClient) InitBlockReceiver(blockHeight uint64, ch chan *alphabill.Block) error {
	return nil
}

func (c *mockAlphaBillClient) SendTransaction(tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	return &transaction.TransactionResponse{Ok: true}, nil
}

func (c *mockAlphaBillClient) Shutdown() {
	// do nothing
}

func (c *mockAlphaBillClient) IsShutdown() bool {
	return false
}
