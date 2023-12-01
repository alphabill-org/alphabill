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

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	"github.com/alphabill-org/alphabill/internal/rpc"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testserver "github.com/alphabill-org/alphabill/internal/testutils/server"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	moneytestutils "github.com/alphabill-org/alphabill/internal/txsystem/money/testutils"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	beclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

var (
	fcrID     = money.NewFeeCreditRecordID(nil, []byte{1})
	fcrAmount = uint64(1e8)
)

func TestCollectDustTimeoutReached(t *testing.T) {
	observe := observability.Default(t)

	// setup account
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, "")
	require.NoError(t, err)
	accKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	// start server
	initialBill := &money.InitialBill{
		ID:    money.NewBillID(nil, []byte{1}),
		Value: 10000 * 1e8,
		Owner: templates.NewP2pkh256BytesFromKey(accKey.PubKey),
	}
	abNet := startMoneyOnlyAlphabillPartition(t, initialBill)
	moneyPart, err := abNet.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	addr := startRPCServer(t, moneyPart)
	serverService := testserver.NewTestAlphabillServiceServer()
	server, _ := testserver.StartServer(serverService, observe.TracerProvider())
	t.Cleanup(server.GracefulStop)

	// start wallet backend
	restAddr := "localhost:9545"
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := backend.Run(ctx,
			&backend.Config{
				ABMoneySystemIdentifier: money.DefaultSystemIdentifier,
				AlphabillUrl:            addr,
				ServerAddr:              restAddr,
				DbFile:                  filepath.Join(t.TempDir(), backend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: backend.InitialBill{
					Id:        initialBill.ID,
					Value:     initialBill.Value,
					Predicate: initialBill.Owner,
				},
				SystemDescriptionRecords: createSDRs(),
				Logger:                   observe.Logger(),
				Observe:                  observe,
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	// setup wallet
	restClient, err := beclient.New(restAddr, observe)
	require.NoError(t, err)
	unitLocker, err := unitlock.NewUnitLocker(dir)
	require.NoError(t, err)
	defer unitLocker.Close()
	feeManagerDB, err := fees.NewFeeManagerDB(dir)
	require.NoError(t, err)
	defer feeManagerDB.Close()
	w, err := LoadExistingWallet(am, unitLocker, feeManagerDB, restClient, observe.Logger())
	require.NoError(t, err)
	defer w.Close()

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(ctx, GetBalanceCmd{})
		return balance == initialBill.Value
	}, test.WaitDuration*2, time.Second)

	// when CollectDust is called
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		_, err := w.CollectDust(context.Background(), 0)
		if err != nil {
			fmt.Println(err)
		}
		wg.Done()
	}()

	for blockNo := uint64(1); blockNo <= txTimeoutBlockCount; blockNo++ {
		b := &types.Block{
			Header: &types.Header{
				SystemID:          w.SystemID(),
				PreviousBlockHash: hash.Sum256([]byte{}),
			},
			Transactions:       []*types.TransactionRecord{},
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNo}},
		}
		serverService.SetBlock(blockNo, b)
	}
	// when dc timeout is reached
	serverService.SetMaxBlockNumber(txTimeoutBlockCount)

	_, err = w.CollectDust(context.Background(), 0)
	require.NoError(t, err)
}

/*
Test scenario:
wallet account 1 sends two bills to wallet accounts 2 and 3
wallet runs dust collection
wallet account 2 and 3 should have only single bill
*/
func TestCollectDustInMultiAccountWallet(t *testing.T) {
	observe := observability.Default(t)

	// setup account
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, "")
	require.NoError(t, err)
	accKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	// start server
	initialBill := &money.InitialBill{
		ID:    money.NewBillID(nil, []byte{1}),
		Value: 10000 * 1e8,
		Owner: templates.NewP2pkh256BytesFromKey(accKey.PubKey),
	}
	network := startMoneyOnlyAlphabillPartition(t, initialBill)
	moneyPart, err := network.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	addr := startRPCServer(t, moneyPart)

	// start wallet backend
	restAddr := "localhost:9545"
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := backend.Run(ctx,
			&backend.Config{
				ABMoneySystemIdentifier: money.DefaultSystemIdentifier,
				AlphabillUrl:            addr,
				ServerAddr:              restAddr,
				DbFile:                  filepath.Join(t.TempDir(), backend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: backend.InitialBill{
					Id:        initialBill.ID,
					Value:     initialBill.Value,
					Predicate: templates.AlwaysTrueBytes(),
				},
				SystemDescriptionRecords: createSDRs(),
				Logger:                   observe.Logger(),
				Observe:                  observe,
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	// setup wallet with multiple keys
	restClient, err := beclient.New(restAddr, observe)
	require.NoError(t, err)
	unitLocker, err := unitlock.NewUnitLocker(dir)
	require.NoError(t, err)
	defer unitLocker.Close()
	feeManagerDB, err := fees.NewFeeManagerDB(dir)
	require.NoError(t, err)
	defer feeManagerDB.Close()
	w, err := LoadExistingWallet(am, unitLocker, feeManagerDB, restClient, observe.Logger())
	require.NoError(t, err)
	defer w.Close()

	_, _, _ = am.AddAccount()
	_, _, _ = am.AddAccount()

	pubKeys, err := am.GetPublicKeys()
	require.NoError(t, err)

	// create fee credit for initial bill transfer
	transferFC := testfc.CreateFeeCredit(t, initialBill.ID, fcrID, fcrAmount, accKey.PrivKey, accKey.PubKey, network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	initialBillValue := initialBill.Value - fcrAmount

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(accKey, initialBill.ID, fcrID, initialBillValue, 10000, initialBillBacklink)
	require.NoError(t, err)
	batch := txsubmitter.NewBatch(accKey.PubKey, w.backend, observe.Logger())
	batch.Add(&txsubmitter.TxSubmission{
		UnitID:      transferInitialBillTx.UnitID(),
		TxHash:      transferInitialBillTx.Hash(crypto.SHA256),
		Transaction: transferInitialBillTx,
	})
	err = batch.SendTx(ctx, false)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPart, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(ctx, GetBalanceCmd{})
		return balance == initialBillValue
	}, test.WaitDuration, time.Second)

	// add fee credit to account 1
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 0,
	})
	require.NoError(t, err)

	// send two bills to account number 2 and 3
	sendTo(t, w, []ReceiverData{
		{Amount: 10 * 1e8, PubKey: pubKeys[1]},
		{Amount: 10 * 1e8, PubKey: pubKeys[1]},
		{Amount: 10 * 1e8, PubKey: pubKeys[2]},
		{Amount: 10 * 1e8, PubKey: pubKeys[2]},
	}, 0)

	// add fee credit to account 2
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 1,
	})
	require.NoError(t, err)

	// add fee credit to account 3
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 2,
	})
	require.NoError(t, err)

	// start dust collection
	_, err = w.CollectDust(ctx, 0)
	require.NoError(t, err)
}

func TestCollectDustInMultiAccountWalletWithKeyFlag(t *testing.T) {
	observe := observability.Default(t)

	// setup account
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, "")
	require.NoError(t, err)
	accKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	// start server
	initialBill := &money.InitialBill{
		ID:    money.NewBillID(nil, []byte{1}),
		Value: 10000 * 1e8,
		Owner: templates.NewP2pkh256BytesFromKey(accKey.PubKey),
	}
	network := startMoneyOnlyAlphabillPartition(t, initialBill)
	moneyPart, err := network.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	addr := startRPCServer(t, moneyPart)

	// start wallet backend
	restAddr := "localhost:9545"
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := backend.Run(ctx,
			&backend.Config{
				ABMoneySystemIdentifier: money.DefaultSystemIdentifier,
				AlphabillUrl:            addr,
				ServerAddr:              restAddr,
				DbFile:                  filepath.Join(t.TempDir(), backend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: backend.InitialBill{
					Id:        initialBill.ID,
					Value:     initialBill.Value,
					Predicate: templates.AlwaysTrueBytes(),
				},
				SystemDescriptionRecords: createSDRs(),
				Logger:                   observe.Logger(),
				Observe:                  observe,
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	// setup wallet with multiple keys
	restClient, err := beclient.New(restAddr, observe)
	require.NoError(t, err)
	unitLocker, err := unitlock.NewUnitLocker(dir)
	require.NoError(t, err)
	defer unitLocker.Close()
	feeManagerDB, err := fees.NewFeeManagerDB(dir)
	require.NoError(t, err)
	defer feeManagerDB.Close()
	w, err := LoadExistingWallet(am, unitLocker, feeManagerDB, restClient, observe.Logger())
	require.NoError(t, err)
	defer w.Close()

	_, _, _ = am.AddAccount()
	_, _, _ = am.AddAccount()

	// transfer initial bill to wallet
	pubKeys, err := am.GetPublicKeys()
	require.NoError(t, err)

	// create fee credit for initial bill transfer
	transferFC := testfc.CreateFeeCredit(t, initialBill.ID, fcrID, fcrAmount, accKey.PrivKey, accKey.PubKey, network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	initialBillValue := initialBill.Value - fcrAmount

	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(accKey, initialBill.ID, fcrID, initialBillValue, 10000, initialBillBacklink)
	require.NoError(t, err)
	batch := txsubmitter.NewBatch(accKey.PubKey, w.backend, observe.Logger())
	batch.Add(&txsubmitter.TxSubmission{
		UnitID:      transferInitialBillTx.UnitID(),
		TxHash:      transferInitialBillTx.Hash(crypto.SHA256),
		Transaction: transferInitialBillTx,
	})
	err = batch.SendTx(ctx, false)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPart, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(ctx, GetBalanceCmd{})
		return balance == initialBillValue
	}, test.WaitDuration, time.Second)

	// add fee credit to wallet account 1
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 0,
	})
	require.NoError(t, err)

	// send two bills to account number 2 and 3
	sendTo(t, w, []ReceiverData{
		{Amount: 10 * 1e8, PubKey: pubKeys[1]},
		{Amount: 10 * 1e8, PubKey: pubKeys[1]},
		{Amount: 10 * 1e8, PubKey: pubKeys[2]},
		{Amount: 10 * 1e8, PubKey: pubKeys[2]},
	}, 0)

	// add fee credit to wallet account 3
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 2,
	})
	require.NoError(t, err)

	// start dust collection only for account number 3
	_, err = w.CollectDust(ctx, 3)
	require.NoError(t, err)

	// verify that there is only one swap tx, and it belongs to account number 3
	account3Key, _ := am.GetAccountKey(2)
	swapTxCount := 0
	testpartition.BlockchainContains(moneyPart, func(txo *types.TransactionOrder) bool {
		if txo.PayloadType() != money.PayloadTypeSwapDC {
			return false
		}

		require.Equal(t, 0, swapTxCount)
		swapTxCount++

		attrs := &money.SwapDCAttributes{}
		err = txo.UnmarshalAttributes(attrs)
		require.NoError(t, err)
		require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(account3Key.PubKeyHash.Sha256), attrs.OwnerCondition)

		return false
	})()
	require.Equal(t, 1, swapTxCount)
}

func sendTo(t *testing.T, w *Wallet, receivers []ReceiverData, fromAccount uint64) {
	proof, err := w.Send(context.Background(), SendCmd{
		Receivers:           receivers,
		AccountIndex:        fromAccount,
		WaitForConfirmation: true,
	})
	require.NoError(t, err)
	require.NotNil(t, proof)
}

func startMoneyOnlyAlphabillPartition(t *testing.T, initialBill *money.InitialBill) *testpartition.AlphabillNetwork {
	mPart, err := testpartition.NewPartition(t, 1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := money.NewTxSystem(
			logger.New(t),
			money.WithSystemIdentifier(money.DefaultSystemIdentifier),
			money.WithInitialBill(initialBill),
			money.WithSystemDescriptionRecords(createSDRs()),
			money.WithDCMoneyAmount(10000*1e8),
			money.WithTrustBase(tb),
		)
		require.NoError(t, err)
		return system
	}, money.DefaultSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{mPart})
	require.NoError(t, err)
	require.NoError(t, abNet.Start(t))
	t.Cleanup(func() { abNet.WaitClose(t) })

	return abNet
}

func startRPCServer(t *testing.T, partition *testpartition.NodePartition) (addr string) {
	// start rpc server for network.Nodes[0]
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer, err := initRPCServer(partition.Nodes[0].Node, observability.Default(t))
	require.NoError(t, err)

	t.Cleanup(func() {
		grpcServer.GracefulStop()
	})
	go func() {
		require.NoError(t, grpcServer.Serve(listener), "gRPC server exited with error")
	}()

	return listener.Addr().String()
}

func initRPCServer(node *partition.Node, obs rpc.Observability) (*grpc.Server, error) {
	rpcServer, err := rpc.NewGRPCServer(node, obs)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer()
	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	return grpcServer, nil
}

func createSDRs() []*genesis.SystemDescriptionRecord {
	return []*genesis.SystemDescriptionRecord{{
		SystemIdentifier: money.DefaultSystemIdentifier,
		T2Timeout:        2500,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         money.NewBillID(nil, []byte{2}),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}}
}
