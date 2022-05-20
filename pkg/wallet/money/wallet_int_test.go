package money

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testserver "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/server"
	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const port = 9111

func TestSync(t *testing.T) {
	// setup wallet
	_ = DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet("", WalletConfig{
		DbPath:                os.TempDir(),
		Db:                    nil,
		AlphabillClientConfig: wallet.AlphabillClientConfig{Uri: "localhost:" + strconv.Itoa(port)},
	})
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	k, err := w.db.Do().GetAccountKey()
	require.NoError(t, err)

	// start server that sends given blocks to wallet
	serviceServer := testserver.NewTestAlphabillServiceServer()
	blocks := []*alphabill.GetBlockResponse{
		{
			Block: &block.Block{
				BlockNumber:       1,
				PreviousBlockHash: hash.Sum256([]byte{}),
				Transactions: []*transaction.Transaction{
					// random dust transfer can be processed
					{
						UnitId:                hash.Sum256([]byte{0x00}),
						TransactionAttributes: testtransaction.CreateRandomDustTransferTx(),
						Timeout:               1000,
						OwnerProof:            script.PredicateArgumentEmpty(),
					},
					// receive transfer of 100 bills
					{
						UnitId:                hash.Sum256([]byte{0x01}),
						TransactionAttributes: testtransaction.CreateBillTransferTx(k.PubKeyHashSha256),
						Timeout:               1000,
						OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
					},
					// receive split of 100 bills
					{
						UnitId:                hash.Sum256([]byte{0x02}),
						TransactionAttributes: testtransaction.CreateBillSplitTx(k.PubKeyHashSha256, 100, 100),
						Timeout:               1000,
						OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
					},
					// receive swap of 100 bills
					{
						UnitId:                hash.Sum256([]byte{0x03}),
						TransactionAttributes: testtransaction.CreateRandomSwapTransferTx(k.PubKeyHashSha256),
						Timeout:               1000,
						OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
					},
				},
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
		},
	}
	serviceServer.SetMaxBlockNumber(1)
	for _, b := range blocks {
		serviceServer.SetBlock(b.Block.BlockNumber, b)
	}
	server := testserver.StartServer(port, serviceServer)
	t.Cleanup(server.GracefulStop)

	// verify starting block number
	blockNumber, err := w.db.Do().GetBlockNumber()
	require.EqualValues(t, 0, blockNumber)
	require.NoError(t, err)

	// verify starting balance
	balance, err := w.GetBalance()
	require.EqualValues(t, 0, balance)
	require.NoError(t, err)

	// when wallet is synced with the node
	go func() {
		_ = w.Sync()
	}()

	// wait for block to be processed
	require.Eventually(t, func() bool {
		blockNo, err := w.db.Do().GetBlockNumber()
		require.NoError(t, err)
		return blockNo == 1
	}, test.WaitDuration, test.WaitTick)

	// then balance is increased
	balance, err = w.GetBalance()
	require.EqualValues(t, 300, balance)
	require.NoError(t, err)
}

func TestSyncToMaxBlockNumber(t *testing.T) {
	// setup wallet
	_ = DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet("", WalletConfig{
		DbPath:                os.TempDir(),
		AlphabillClientConfig: wallet.AlphabillClientConfig{Uri: "localhost:" + strconv.Itoa(port)}},
	)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	// start server that sends given blocks to wallet
	serviceServer := testserver.NewTestAlphabillServiceServer()
	maxBlockNumber := uint64(3)
	for blockNo := uint64(1); blockNo <= 10; blockNo++ {
		b := &alphabill.GetBlockResponse{
			Block: &block.Block{
				BlockNumber:        blockNo,
				PreviousBlockHash:  hash.Sum256([]byte{}),
				Transactions:       []*transaction.Transaction{},
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
		}
		serviceServer.SetBlock(blockNo, b)
	}
	serviceServer.SetMaxBlockNumber(maxBlockNumber)
	server := testserver.StartServer(port, serviceServer)
	t.Cleanup(server.GracefulStop)

	// verify starting block number
	blockNumber, err := w.db.Do().GetBlockNumber()
	require.EqualValues(t, 0, blockNumber)
	require.NoError(t, err)

	// when wallet is synced to max block number
	err = w.SyncToMaxBlockNumber()
	require.NoError(t, err)

	// then block number is exactly equal to max block number, and further blocks are not processed
	blockNumber, err = w.db.Do().GetBlockNumber()
	require.EqualValues(t, maxBlockNumber, blockNumber)
	require.NoError(t, err)
}

func TestCollectDustTimeoutReached(t *testing.T) {
	// setup wallet
	_ = DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet("", WalletConfig{
		DbPath:                os.TempDir(),
		AlphabillClientConfig: wallet.AlphabillClientConfig{Uri: "localhost:" + strconv.Itoa(port)},
	})
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)
	addBill(t, w, 100)
	addBill(t, w, 200)

	// start server
	serverService := testserver.NewTestAlphabillServiceServer()
	server := testserver.StartServer(port, serverService)
	t.Cleanup(server.GracefulStop)

	// when CollectDust is called
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = w.CollectDust()
		if err != nil {
			fmt.Println(err)
		}
		wg.Done()
	}()

	// and wallet synchronization is started
	go w.Sync()

	// then dc transactions are sent
	waitForExpectedSwap(w)
	require.Len(t, serverService.GetProcessedTransactions(), 2)
	require.NoError(t, err)

	// and dc wg metadata is saved
	require.Len(t, w.dcWg.swaps, 1)
	dcNonce := calculateExpectedDcNonce(t, w)
	require.EqualValues(t, w.dcWg.swaps[*uint256.NewInt(0).SetBytes(dcNonce)], dcTimeoutBlockCount)

	// when dc timeout is reached
	serverService.SetMaxBlockNumber(dcTimeoutBlockCount)
	for blockNo := uint64(1); blockNo <= dcTimeoutBlockCount; blockNo++ {
		b := &alphabill.GetBlockResponse{
			Block: &block.Block{
				BlockNumber:        blockNo,
				PreviousBlockHash:  hash.Sum256([]byte{}),
				Transactions:       []*transaction.Transaction{},
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
		}
		serverService.SetBlock(blockNo, b)
	}

	// then collect dust should finish
	wg.Wait()

	// and dc wg is cleared
	require.Len(t, w.dcWg.swaps, 0)
}
