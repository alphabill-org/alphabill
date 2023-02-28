package money

import (
	"context"
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestWalletBackend_BillsCanBeIndexedByPredicates(t *testing.T) {
	// create wallet backend with mock abclient
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	billId1 := newUnitID(1)
	billId2 := newUnitID(2)
	pubkey1, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubkey2, _ := hexutil.Decode("0x02c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	bearer1 := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey1))
	bearer2 := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey2))

	abclient := clientmock.NewMockAlphabillClient(
		clientmock.WithMaxBlockNumber(1),
		clientmock.WithBlocks(map[uint64]*block.Block{
			1: {
				UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
				Transactions: []*txsystem.Transaction{{
					UnitId:                billId1,
					SystemId:              moneySystemID,
					TransactionAttributes: moneytesttx.CreateBillTransferTx(hash.Sum256(pubkey1)),
				}},
			},
			2: {
				UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 2}},
				Transactions: []*txsystem.Transaction{{
					UnitId:                billId2,
					SystemId:              moneySystemID,
					TransactionAttributes: moneytesttx.CreateBillTransferTx(hash.Sum256(pubkey2)),
				}},
			},
		}))
	w := createWalletBackend(t, abclient)

	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := w.Start(ctx)
		require.NoError(t, err)
	}()

	// verify first unit is indexed
	require.Eventually(t, func() bool {
		bills, err := w.store.Do().GetBills(bearer1)
		require.NoError(t, err)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)

	// serve block with transaction to new pubkey
	abclient.SetMaxBlockNumber(2)

	// verify new bill is indexed by pubkey
	require.Eventually(t, func() bool {
		bills, err := w.store.Do().GetBills(bearer2)
		require.NoError(t, err)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)
}

func TestGetBills_OK(t *testing.T) {
	txValue := uint64(100)
	pubkey := make([]byte, 32)
	bearer := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey))
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
		NewBearer:   bearer,
	}))
	gtx, err := backend.NewTxConverter(moneySystemID).ConvertTx(tx)
	require.NoError(t, err)
	txHash := gtx.Hash(gocrypto.SHA256)

	store, err := createTestBillStore(t)
	require.NoError(t, err)

	// add bill to service
	service := New(nil, store)
	b := &Bill{
		Id:             tx.UnitId,
		Value:          txValue,
		TxHash:         txHash,
		OrderNumber:    1,
		OwnerPredicate: bearer,
		TxProof: &TxProof{
			BlockNumber: 1,
			Tx:          tx,
		},
	}
	err = store.Do().SetBill(b)
	require.NoError(t, err)

	// verify bill can be queried by id
	bill, err := service.GetBill(b.Id)
	require.NoError(t, err)
	require.Equal(t, b, bill)

	// verify bill can be queried by pubkey
	bills, err := service.GetBills(pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, b, bills[0])
}

func TestGetBills_SHA512OK(t *testing.T) {
	txValue := uint64(100)
	pubkey := make([]byte, 32)
	bearer := script.PredicatePayToPublicKeyHashDefault(hash.Sum512(pubkey))
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
		NewBearer:   bearer,
	}))

	store, err := createTestBillStore(t)
	require.NoError(t, err)

	// add sha512 owner condition bill to service
	service := New(nil, store)
	b := &Bill{
		Id:             tx.UnitId,
		Value:          txValue,
		OrderNumber:    1,
		OwnerPredicate: bearer,
	}
	err = store.Do().SetBill(b)
	require.NoError(t, err)

	// verify bill can be queried by pubkey
	bills, err := service.GetBills(pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, b, bills[0])
}

func createWalletBackend(t *testing.T, abclient client.ABClient) *WalletBackend {
	storage, _ := createTestBillStore(t)
	bp := NewBlockProcessor(storage, backend.NewTxConverter(moneySystemID))
	genericWallet := wallet.New().SetBlockProcessor(bp).SetABClient(abclient).Build()
	return New(genericWallet, storage)
}
