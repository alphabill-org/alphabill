package pubkey_indexer

import (
	"context"
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
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

func TestWalletBackend_BillsCanBeIndexedByPubkeys(t *testing.T) {
	// create wallet backend with mock abclient
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	billId1 := newUnitID(1)
	pubKey1, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	billId2 := newUnitID(2)
	pubkey2, _ := hexutil.Decode("0x02c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")

	abclient := clientmock.NewMockAlphabillClient(1, map[uint64]*block.Block{
		1: {
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
			Transactions: []*txsystem.Transaction{{
				UnitId:                billId1,
				SystemId:              moneySystemID,
				TransactionAttributes: moneytesttx.CreateBillTransferTx(hash.Sum256(pubKey1)),
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
	})
	w := createWalletBackend(t, abclient)
	err := w.AddKey(pubKey1)
	require.NoError(t, err)

	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := w.Start(ctx)
		require.NoError(t, err)
	}()

	// verify first pubkey is indexed
	require.Eventually(t, func() bool {
		ok, _ := w.store.ContainsBill(pubKey1, billId1)
		return ok
	}, test.WaitDuration, test.WaitTick)

	// add new pubkey to indexer
	err = w.AddKey(pubkey2)
	require.NoError(t, err)

	// and serve block with transaction to new pubkey
	abclient.SetMaxBlockNumber(2)

	// verify new bill is indexed by pubkey
	// TODO fix flaky test, possible cause abclient not thread safe?
	require.Eventually(t, func() bool {
		ok, _ := w.store.ContainsBill(pubkey2, billId2)
		return ok
	}, test.WaitDuration, test.WaitTick)
}

func TestSetBill_OK(t *testing.T) {
	txValue := uint64(100)
	pubkey := make([]byte, 32)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey)),
	}))
	txConverter := backend.NewTxConverter(moneySystemID)
	gtx, _ := txConverter.ConvertTx(tx)
	txHash := gtx.Hash(gocrypto.SHA256)
	proof, verifiers := createProofForTx(t, tx)

	store, _ := createTestBillStore(t)
	service := New(nil, store, txConverter, verifiers)
	b := &Bill{
		Id:          tx.UnitId,
		Value:       txValue,
		TxHash:      txHash,
		OrderNumber: 1,
		TxProof: &TxProof{
			BlockNumber: 1,
			Tx:          tx,
			Proof:       proof,
		},
	}
	protoBills := b.toProtoBills()

	// verify bill cannot be added to untracked pubkey
	err := service.SetBills(pubkey, protoBills)
	require.ErrorIs(t, err, errKeyNotIndexed)

	// when pubkey is indexed
	err = service.AddKey(pubkey)
	require.NoError(t, err)

	// then bills can be added for given pubkey
	err = service.SetBills(pubkey, protoBills)
	require.NoError(t, err)

	// verify saved bill can be queried by id
	bill, err := service.GetBill(pubkey, tx.UnitId)
	require.NoError(t, err)
	require.Equal(t, b, bill)

	// verify saved bill can be queried by pubkey
	bills, err := service.GetBills(pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, b, bills[0])
}

func TestSetBill_InvalidProof_NOK(t *testing.T) {
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
	}))
	txConverter := backend.NewTxConverter(moneySystemID)
	gtx, _ := txConverter.ConvertTx(tx)
	txHash := gtx.Hash(gocrypto.SHA256)
	proof, verifiers := createProofForTx(t, tx)
	proof.BlockHeaderHash = make([]byte, 32) // invalidate proof

	store, _ := createTestBillStore(t)
	service := New(nil, store, txConverter, verifiers)
	pubkey := []byte{0}
	_ = service.AddKey(pubkey)
	b := &Bill{
		Id:     tx.UnitId,
		Value:  txValue,
		TxHash: txHash,
		TxProof: &TxProof{
			BlockNumber: 1,
			Tx:          tx,
			Proof:       proof,
		},
	}
	err := service.SetBills(pubkey, b.toProtoBills())
	require.ErrorIs(t, err, block.ErrProofVerificationFailed)
}

func createWalletBackend(t *testing.T, abclient client.ABClient) *WalletBackend {
	storage, _ := createTestBillStore(t)
	txConverter := backend.NewTxConverter(moneySystemID)
	bp := NewBlockProcessor(storage, txConverter)
	genericWallet := wallet.New().SetBlockProcessor(bp).SetABClient(abclient).Build()
	_, verifier := testsig.CreateSignerAndVerifier(t)
	return New(genericWallet, storage, txConverter, map[string]crypto.Verifier{"test": verifier})
}
