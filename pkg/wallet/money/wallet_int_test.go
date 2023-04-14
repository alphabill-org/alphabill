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
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/rpc"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testmoney "github.com/alphabill-org/alphabill/internal/testutils/money"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testserver "github.com/alphabill-org/alphabill/internal/testutils/server"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	moneytestutils "github.com/alphabill-org/alphabill/internal/txsystem/money/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
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
)

func TestCollectDustTimeoutReached(t *testing.T) {
	t.SkipNow() // TODO add fee handling to money wallet

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

	// start wallet backend
	restAddr := "localhost:9545"
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
				InitialBill: money.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: script.PredicateAlwaysTrue(),
				},
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

	// create fee credit for initial bill transfer
	txFee := fc.FixedFee(1)()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	initialBillValue := initialBill.Value - fcrAmount - txFee

	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(pubKeys[0], initialBill.ID, initialBillValue, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = w.SendTransaction(ctx, transferInitialBillTx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(GetBalanceCmd{})
		return balance == initialBillValue
	}, test.WaitDuration*2, time.Second)

	// when CollectDust is called
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = w.CollectDust(context.Background(), 0)
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

	err = w.CollectDust(context.Background(), 0)

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
	t.SkipNow() // TODO add fee handling to money wallet

	// start network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	addr := "localhost:9544"
	startRPCServer(t, network, addr)

	// start wallet backend
	restAddr := "localhost:9545"
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
				InitialBill: money.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: script.PredicateAlwaysTrue(),
				},
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

	// create fee credit for initial bill transfer
	txFee := fc.FixedFee(1)()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	initialBillValue := initialBill.Value - fcrAmount - txFee

	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(pubKeys[0], initialBill.ID, initialBillValue, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = w.SendTransaction(ctx, transferInitialBillTx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(GetBalanceCmd{})
		return balance == initialBillValue
	}, test.WaitDuration, time.Second)

	// TODO add fee credit support to "new" cli wallet

	// send two bills to account number 2 and 3
	sendToAccount(t, w, 10, 0, 1)
	sendToAccount(t, w, 10, 0, 1)
	sendToAccount(t, w, 10, 0, 2)
	sendToAccount(t, w, 10, 0, 2)

	// start dust collection
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		err := w.CollectDust(ctx, 0)
		if err == nil {
			defer cancel() // signal Sync to cancel
		}
		return err
	})
}

func TestCollectDustTimeoutReached_WithoutFees(t *testing.T) {
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

	// start wallet backend
	restAddr := "localhost:9545"
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
				InitialBill: money.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: script.PredicateAlwaysTrue(),
				},
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

	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(pubKeys[0], initialBill.ID, initialBill.Value, 10000, nil)
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
		err = w.CollectDust(context.Background(), 0)
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

	err = w.CollectDust(context.Background(), 0)

	// and dc wg is cleared
	require.Len(t, w.dcWg.swaps, 0)
}

/*
Test scenario:
wallet account 1 sends two bills to wallet accounts 2 and 3
wallet runs dust collection
wallet account 2 and 3 should have only single bill
*/
func TestCollectDustInMultiAccountWallet_WithoutFees(t *testing.T) {
	// start network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	addr := "localhost:9544"
	startRPCServer(t, network, addr)

	// start wallet backend
	restAddr := "localhost:9545"
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
				InitialBill: money.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: script.PredicateAlwaysTrue(),
				},
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

	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(pubKeys[0], initialBill.ID, initialBill.Value, 10000, nil)
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
	sendToAccount(t, w, 10, 0, 1)
	sendToAccount(t, w, 10, 0, 1)
	sendToAccount(t, w, 10, 0, 2)
	sendToAccount(t, w, 10, 0, 2)

	// start dust collection
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		err := w.CollectDust(ctx, 0)
		if err == nil {
			defer cancel() // signal Sync to cancel
		}
		return err
	})
}

func sendToAccount(t *testing.T, w *Wallet, amount, fromAccount, toAccount uint64) {
	receiverPubkey, err := w.am.GetPublicKey(toAccount)
	require.NoError(t, err)

	prevBalance, err := w.GetBalance(GetBalanceCmd{AccountIndex: toAccount})
	require.NoError(t, err)

	_, err = w.Send(context.Background(), SendCmd{ReceiverPubKey: receiverPubkey, Amount: amount, AccountIndex: fromAccount})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(GetBalanceCmd{AccountIndex: toAccount})
		return balance > prevBalance
	}, test.WaitDuration, time.Second)
}

func startAlphabillPartition(t *testing.T, initialBill *moneytx.InitialBill) *testpartition.AlphabillPartition {
	network, err := testpartition.NewNetwork(1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := moneytx.NewMoneyTxSystem(
			[]byte{0, 0, 0, 0},
			moneytx.WithInitialBill(initialBill),
			moneytx.WithSystemDescriptionRecords(createSDRs(2)),
			moneytx.WithDCMoneyAmount(10000),
			moneytx.WithTrustBase(tb),
			moneytx.WithFeeCalculator(fc.FixedFee(0)), // 0 to disable fee module
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

	grpcServer, err := initRPCServer(network.Nodes[0].Node)
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

func createSDRs(unitID uint64) []*genesis.SystemDescriptionRecord {
	return []*genesis.SystemDescriptionRecord{{
		SystemIdentifier: []byte{0, 0, 0, 0},
		T2Timeout:        2500,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         util.Uint256ToBytes(uint256.NewInt(unitID)),
			OwnerPredicate: script.PredicateAlwaysTrue(),
		},
	}}
}
