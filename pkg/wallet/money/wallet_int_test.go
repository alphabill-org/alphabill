package money

import (
	"context"
	"crypto"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/rpc"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testserver "github.com/alphabill-org/alphabill/internal/testutils/server"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	testclient "github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestCollectDustTimeoutReached(t *testing.T) {
	// start server
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	addr := "localhost:9544"
	startRPCServer(t, network, addr)
	serverService := testserver.NewTestAlphabillServiceServer()
	server, _ := testserver.StartServer(serverService)
	t.Cleanup(server.GracefulStop)

	restAddr := "localhost:9545"
	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := money.CreateAndRun(ctx,
			&money.Config{
				ABMoneySystemIdentifier: []byte{0, 0, 0, 0},
				AlphabillUrl:            addr,
				ServerAddr:              restAddr,
				DbFile:                  filepath.Join(t.TempDir(), money.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	// setup wallet
	_ = log.InitStdoutLogger(log.DEBUG)
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, "")
	require.NoError(t, err)
	restClient, err := testclient.NewClient(restAddr)
	require.NoError(t, err)
	w, err := LoadExistingWallet(client.AlphabillClientConfig{Uri: addr}, am, restClient)
	require.NoError(t, err)
	pubKeys, err := am.GetPublicKeys()
	require.NoError(t, err)

	transferInitialBillTx, err := createInitialBillTransferTx(pubKeys[0], initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = w.SendTransaction(ctx, transferInitialBillTx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(GetBalanceCmd{})
		return balance == initialBill.Value
	}, test.WaitDuration*2, time.Second)

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

	// then dc transactions are sent
	waitForExpectedSwap(w)

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

	err = w.CollectDust(context.Background())

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

	restAddr := "localhost:9545"
	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := money.CreateAndRun(ctx,
			&money.Config{
				ABMoneySystemIdentifier: []byte{0, 0, 0, 0},
				AlphabillUrl:            addr,
				ServerAddr:              restAddr,
				DbFile:                  filepath.Join(t.TempDir(), money.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	// setup wallet with multiple keys
	_ = log.InitStdoutLogger(log.DEBUG)
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, "")
	require.NoError(t, err)
	restClient, err := testclient.NewClient(restAddr)
	require.NoError(t, err)
	w, err := LoadExistingWallet(client.AlphabillClientConfig{Uri: addr}, am, restClient)
	require.NoError(t, err)

	_, _, _ = am.AddAccount()
	_, _, _ = am.AddAccount()

	// transfer initial bill to wallet 1
	pubKeys, err := am.GetPublicKeys()
	require.NoError(t, err)

	transferInitialBillTx, err := createInitialBillTransferTx(pubKeys[0], initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = w.SendTransaction(ctx, transferInitialBillTx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(GetBalanceCmd{})
		return balance == initialBill.Value
	}, test.WaitDuration, time.Second)

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
		err := w.CollectDust(ctx)
		if err == nil {
			defer cancel() // signal Sync to cancel
		}
		return err
	})
}

func sendToAccount(t *testing.T, w *Wallet, accountIndexTo uint64) {
	receiverPubkey, err := w.am.GetPublicKey(accountIndexTo)
	require.NoError(t, err)

	prevBalance, err := w.GetBalance(GetBalanceCmd{AccountIndex: accountIndexTo})
	require.NoError(t, err)

	_, err = w.Send(context.Background(), SendCmd{ReceiverPubKey: receiverPubkey, Amount: 1})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(GetBalanceCmd{AccountIndex: accountIndexTo})
		return balance > prevBalance
	}, test.WaitDuration, time.Second)
}

func startAlphabillPartition(t *testing.T, initialBill *moneytx.InitialBill) *testpartition.AlphabillPartition {
	network, err := testpartition.NewNetwork(1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := moneytx.NewMoneyTxSystem(
			crypto.SHA256,
			initialBill,
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
