package money

import (
	"context"
	"crypto"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/rpc"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testserver "github.com/alphabill-org/alphabill/internal/testutils/server"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const port = 9111

func TestSync(t *testing.T) {
	// setup wallet
	_ = DeleteWalletDb(os.TempDir())
	_ = log.InitStdoutLogger(log.DEBUG)
	w, err := CreateNewWallet("", WalletConfig{
		DbPath:                os.TempDir(),
		Db:                    nil,
		AlphabillClientConfig: client.AlphabillClientConfig{Uri: "localhost:" + strconv.Itoa(port)},
	})
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	k, err := w.db.Do().GetAccountKey(0)
	require.NoError(t, err)

	// start server that sends given blocks to wallet
	serviceServer := testserver.NewTestAlphabillServiceServer()
	blocks := []*block.Block{
		{
			SystemIdentifier:  w.SystemID(),
			PreviousBlockHash: hash.Sum256([]byte{}),
			Transactions: []*txsystem.Transaction{
				// random dust transfer can be processed
				{
					SystemId:              w.SystemID(),
					UnitId:                hash.Sum256([]byte{0x00}),
					TransactionAttributes: moneytesttx.CreateRandomDustTransferTx(),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentEmpty(),
				},
				// receive transfer of 100 bills
				{
					SystemId:              w.SystemID(),
					UnitId:                hash.Sum256([]byte{0x01}),
					TransactionAttributes: moneytesttx.CreateBillTransferTx(k.PubKeyHash.Sha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive split of 100 bills
				{
					SystemId:              w.SystemID(),
					UnitId:                hash.Sum256([]byte{0x02}),
					TransactionAttributes: moneytesttx.CreateBillSplitTx(k.PubKeyHash.Sha256, 100, 100),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive swap of 100 bills
				{
					SystemId:              w.SystemID(),
					UnitId:                hash.Sum256([]byte{0x03}),
					TransactionAttributes: moneytesttx.CreateRandomSwapTransferTx(k.PubKeyHash.Sha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
			},
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
		},
	}
	serviceServer.SetMaxBlockNumber(1)
	for _, b := range blocks {
		serviceServer.SetBlock(b.UnicityCertificate.InputRecord.RoundNumber, b)
	}
	server := testserver.StartServer(port, serviceServer)
	t.Cleanup(server.GracefulStop)

	// verify starting block number
	blockNumber, err := w.db.Do().GetBlockNumber()
	require.EqualValues(t, 0, blockNumber)
	require.NoError(t, err)

	// verify starting balance
	balance, err := w.GetBalance(GetBalanceCmd{})
	require.EqualValues(t, 0, balance)
	require.NoError(t, err)

	// when wallet is synced with the node
	go func() {
		_ = w.Sync(context.Background())
	}()

	// wait for block to be processed
	require.Eventually(t, func() bool {
		blockNo, err := w.db.Do().GetBlockNumber()
		require.NoError(t, err)
		return blockNo == 1
	}, test.WaitDuration, test.WaitTick)

	// then balance is increased
	balance, err = w.GetBalance(GetBalanceCmd{})
	require.EqualValues(t, 300, balance)
	require.NoError(t, err)
}

func TestSyncToMaxBlockNumber(t *testing.T) {
	// setup wallet
	_ = DeleteWalletDb(os.TempDir())
	_ = log.InitStdoutLogger(log.DEBUG)
	w, err := CreateNewWallet("", WalletConfig{
		DbPath:                os.TempDir(),
		AlphabillClientConfig: client.AlphabillClientConfig{Uri: "localhost:" + strconv.Itoa(port)}},
	)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	// start server that sends given blocks to wallet
	serviceServer := testserver.NewTestAlphabillServiceServer()
	maxBlockNumber := uint64(3)
	for blockNo := uint64(1); blockNo <= 10; blockNo++ {
		b := &block.Block{
			SystemIdentifier:   w.SystemID(),
			PreviousBlockHash:  hash.Sum256([]byte{}),
			Transactions:       []*txsystem.Transaction{},
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: blockNo}},
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
	err = w.SyncToMaxBlockNumber(context.Background())
	require.NoError(t, err)

	// then block number is exactly equal to max block number, and further blocks are not processed
	blockNumber, err = w.db.Do().GetBlockNumber()
	require.EqualValues(t, maxBlockNumber, blockNumber)
	require.NoError(t, err)
}

func TestCollectDustTimeoutReached(t *testing.T) {
	// setup wallet
	_ = log.InitStdoutLogger(log.DEBUG)
	_ = DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet("", WalletConfig{
		DbPath:                os.TempDir(),
		AlphabillClientConfig: client.AlphabillClientConfig{Uri: "localhost:" + strconv.Itoa(port)},
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
		err = w.CollectDust(context.Background())
		if err != nil {
			fmt.Println(err)
		}
		wg.Done()
	}()

	// and wallet synchronization is started
	go func() {
		err := w.Sync(context.Background())
		if err != nil {
			log.Warning("Wallet sync failed: ", err)
		}
	}()

	// then dc transactions are sent
	waitForExpectedSwap(w)
	require.Len(t, serverService.GetProcessedTransactions(), 2)
	require.NoError(t, err)

	// and dc wg metadata is saved
	require.Len(t, w.dcWg.swaps, 1)
	dcNonce := calculateExpectedDcNonce(t, w)
	require.EqualValues(t, w.dcWg.swaps[*uint256.NewInt(0).SetBytes(dcNonce)], dcTimeoutBlockCount)

	for blockNo := uint64(1); blockNo <= dcTimeoutBlockCount; blockNo++ {
		b := &block.Block{
			SystemIdentifier:   w.SystemID(),
			PreviousBlockHash:  hash.Sum256([]byte{}),
			Transactions:       []*txsystem.Transaction{},
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: blockNo}},
		}
		serverService.SetBlock(blockNo, b)
	}
	// when dc timeout is reached
	serverService.SetMaxBlockNumber(dcTimeoutBlockCount)

	// then collect dust should finish
	wg.Wait()

	// and dc wg is cleared
	require.Len(t, w.dcWg.swaps, 0)
}

/*
Test scenario:
wallet account 1 sends two bills to wallet accounts 2 and 3
wallet runs dust collection
wallet account 2 and 3 should have only single bill
*/
func TestCollectDustInMultiAccountWallet(t *testing.T) {
	// start network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	addr := "localhost:9544"
	startRPCServer(t, network, addr)

	// setup wallet with multiple keys
	_ = log.InitStdoutLogger(log.DEBUG)
	_ = DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet("", WalletConfig{
		DbPath:                os.TempDir(),
		AlphabillClientConfig: client.AlphabillClientConfig{Uri: addr},
	})
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	_, _, _ = w.AddAccount()
	_, _, _ = w.AddAccount()

	// transfer initial bill to wallet 1
	pubkeys, err := w.GetPublicKeys()
	require.NoError(t, err)

	transferInitialBillTx, err := createInitialBillTransferTx(pubkeys[0], initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	err = w.SyncToMaxBlockNumber(context.Background())
	require.NoError(t, err)
	balance, err := w.GetBalance(GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, initialBill.Value, balance)

	// send two bills to account number 2 and 3
	sendToAccount(t, w, 1)
	sendToAccount(t, w, 1)
	sendToAccount(t, w, 2)
	sendToAccount(t, w, 2)

	// start dust collection
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return w.Sync(ctx)
	})
	group.Go(func() error {
		err := w.CollectDust(ctx)
		if err == nil {
			defer cancel() // signal Sync to cancel
		}
		return err
	})

	// wait for dust collection to finish
	err = group.Wait()
	require.NoError(t, err)

	// verify all accounts have single bill with expected value
	for accountIndex := uint64(0); accountIndex < 3; accountIndex++ {
		bills, err := w.db.Do().GetBills(accountIndex)
		require.NoError(t, err)
		require.Len(t, bills, 1)
	}
}

func sendToAccount(t *testing.T, w *Wallet, accountIndexTo uint64) {
	receiverPubkey, err := w.GetPublicKey(accountIndexTo)
	require.NoError(t, err)

	prevBalance, err := w.GetBalance(GetBalanceCmd{AccountIndex: accountIndexTo})
	require.NoError(t, err)

	_, err = w.Send(context.Background(), SendCmd{ReceiverPubKey: receiverPubkey, Amount: 1})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_ = w.SyncToMaxBlockNumber(context.Background())
		balance, _ := w.GetBalance(GetBalanceCmd{AccountIndex: accountIndexTo})
		return balance > prevBalance
	}, test.WaitDuration, time.Second)
}

func startAlphabillPartition(t *testing.T, initialBill *moneytx.InitialBill) *testpartition.AlphabillPartition {
	network, err := testpartition.NewNetwork(1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := moneytx.NewMoneyTxSystem(
			crypto.SHA256,
			initialBill,
			createSDRs(2),
			10000,
			moneytx.SchemeOpts.TrustBase(tb),
		)
		require.NoError(t, err)
		return system
	}, []byte{0, 0, 0, 0})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = network.Close()
	})
	return network
}

func startRPCServer(t *testing.T, network *testpartition.AlphabillPartition, addr string) {
	// start rpc server for network.Nodes[0]
	listener, err := net.Listen("tcp", addr)
	require.NoError(t, err)

	grpcServer, err := initRPCServer(network.Nodes[0])
	require.NoError(t, err)

	t.Cleanup(func() {
		grpcServer.GracefulStop()
	})
	go func() {
		_ = grpcServer.Serve(listener)
	}()
}

func initRPCServer(node *partition.Node) (*grpc.Server, error) {
	grpcServer := grpc.NewServer()
	rpcServer, err := rpc.NewGRPCServer(node)
	if err != nil {
		return nil, err
	}
	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	return grpcServer, nil
}

func createInitialBillTransferTx(pubKey []byte, billId *uint256.Int, billValue uint64, timeout uint64) (*txsystem.Transaction, error) {
	billId32 := billId.Bytes32()
	tx := &txsystem.Transaction{
		UnitId:                billId32[:],
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            script.PredicateArgumentEmpty(),
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &moneytx.TransferOrder{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: billValue,
		Backlink:    nil,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createSDRs(id uint64) []*genesis.SystemDescriptionRecord {
	return []*genesis.SystemDescriptionRecord{{
		SystemIdentifier: alphabillMoneySystemId,
		T2Timeout:        2500,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         util.Uint256ToBytes(uint256.NewInt(id)),
			OwnerPredicate: script.PredicateAlwaysTrue(),
		},
	}}
}
