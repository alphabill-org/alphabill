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
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	abclient "github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	beclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

var moneySysId = []byte{0, 0, 0, 0}

func TestCollectDustTimeoutReached(t *testing.T) {
	// start server
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000 * 1e8,
		Owner: script.PredicateAlwaysTrue(),
	}
	abNet := startMoneyOnlyAlphabillPartition(t, initialBill)
	moneyPart, err := abNet.GetNodePartition(moneySysId)
	require.NoError(t, err)
	addr := startRPCServer(t, moneyPart)
	serverService := testserver.NewTestAlphabillServiceServer()
	server, _ := testserver.StartServer(serverService)
	t.Cleanup(server.GracefulStop)

	// start wallet backend
	restAddr := "localhost:9545"
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := backend.Run(ctx,
			&backend.Config{
				ABMoneySystemIdentifier: moneytx.DefaultSystemIdentifier,
				AlphabillUrl:            addr,
				ServerAddr:              restAddr,
				DbFile:                  filepath.Join(t.TempDir(), backend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: backend.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: script.PredicateAlwaysTrue(),
				},
				SystemDescriptionRecords: createSDRs(2),
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
	restClient, err := beclient.New(restAddr)
	require.NoError(t, err)
	w, err := LoadExistingWallet(abclient.AlphabillClientConfig{Uri: addr}, am, restClient)
	require.NoError(t, err)
	pubKeys, err := am.GetPublicKeys()
	require.NoError(t, err)

	// create fee credit for initial bill transfer
	txFee := fc.FixedFee(1)()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), abNet)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	initialBillValue := initialBill.Value - fcrAmount - txFee

	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(pubKeys[0], initialBill.ID, initialBillValue, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = w.SendTransaction(ctx, transferInitialBillTx, &wallet.SendOpts{})
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPart, transferInitialBillTx), test.WaitDuration, test.WaitTick)

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
	// start network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000 * 1e8,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startMoneyOnlyAlphabillPartition(t, initialBill)
	moneyPart, err := network.GetNodePartition(moneySysId)
	require.NoError(t, err)
	addr := startRPCServer(t, moneyPart)

	// start wallet backend
	restAddr := "localhost:9545"
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := backend.Run(ctx,
			&backend.Config{
				ABMoneySystemIdentifier: []byte{0, 0, 0, 0},
				AlphabillUrl:            addr,
				ServerAddr:              restAddr,
				DbFile:                  filepath.Join(t.TempDir(), backend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: backend.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: script.PredicateAlwaysTrue(),
				},
				SystemDescriptionRecords: createSDRs(2),
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
	restClient, err := beclient.New(restAddr)
	require.NoError(t, err)
	w, err := LoadExistingWallet(abclient.AlphabillClientConfig{Uri: addr}, am, restClient)
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
	err = w.SendTransaction(ctx, transferInitialBillTx, &wallet.SendOpts{})
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPart, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(GetBalanceCmd{})
		return balance == initialBillValue
	}, test.WaitDuration, time.Second)

	// add fee credit to account 1
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 0,
	})
	require.NoError(t, err)

	// send two bills to account number 2 and 3
	sendToAccount(t, w, 10*1e8, 0, 1)
	sendToAccount(t, w, 10*1e8, 0, 1)
	sendToAccount(t, w, 10*1e8, 0, 2)
	sendToAccount(t, w, 10*1e8, 0, 2)

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
	err = w.CollectDust(ctx, 0)
	require.NoError(t, err)
}

func TestCollectDustInMultiAccountWalletWithKeyFlag(t *testing.T) {
	// start network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000 * 1e8,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startMoneyOnlyAlphabillPartition(t, initialBill)
	moneyPart, err := network.GetNodePartition(moneySysId)
	require.NoError(t, err)
	addr := startRPCServer(t, moneyPart)

	// start wallet backend
	restAddr := "localhost:9545"
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := backend.Run(ctx,
			&backend.Config{
				ABMoneySystemIdentifier: []byte{0, 0, 0, 0},
				AlphabillUrl:            addr,
				ServerAddr:              restAddr,
				DbFile:                  filepath.Join(t.TempDir(), backend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: backend.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: script.PredicateAlwaysTrue(),
				},
				SystemDescriptionRecords: createSDRs(2),
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
	restClient, err := beclient.New(restAddr)
	require.NoError(t, err)
	w, err := LoadExistingWallet(abclient.AlphabillClientConfig{Uri: addr}, am, restClient)
	require.NoError(t, err)

	_, _, _ = am.AddAccount()
	_, _, _ = am.AddAccount()

	// transfer initial bill to wallet
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
	err = w.SendTransaction(ctx, transferInitialBillTx, &wallet.SendOpts{})
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPart, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(GetBalanceCmd{})
		return balance == initialBillValue
	}, test.WaitDuration, time.Second)

	// add fee credit to wallet account 1
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 0,
	})
	require.NoError(t, err)

	// send two bills to account number 2 and 3
	sendToAccount(t, w, 10*1e8, 0, 1)
	sendToAccount(t, w, 10*1e8, 0, 1)
	sendToAccount(t, w, 10*1e8, 0, 2)
	sendToAccount(t, w, 10*1e8, 0, 2)

	// add fee credit to wallet account 3
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 2,
	})
	require.NoError(t, err)

	// start dust collection only for account number 3
	err = w.CollectDust(ctx, 3)
	require.NoError(t, err)

	// verify that there is only one swap tx, and it belongs to account number 3
	b, _ := moneyPart.Nodes[0].GetLatestBlock()
	require.Len(t, b.Transactions, 1)
	attrs := &moneytx.SwapDCAttributes{}
	err = b.Transactions[0].TransactionOrder.UnmarshalAttributes(attrs)
	require.NoError(t, err)
	k, _ := am.GetAccountKey(2)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), attrs.OwnerCondition)
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

func startMoneyOnlyAlphabillPartition(t *testing.T, initialBill *moneytx.InitialBill) *testpartition.AlphabillNetwork {
	mPart, err := testpartition.NewPartition(1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := moneytx.NewTxSystem(
			moneytx.WithSystemIdentifier(moneytx.DefaultSystemIdentifier),
			moneytx.WithInitialBill(initialBill),
			moneytx.WithSystemDescriptionRecords(createSDRs(2)),
			moneytx.WithDCMoneyAmount(10000*1e8),
			moneytx.WithTrustBase(tb),
		)
		require.NoError(t, err)
		return system
	}, moneySysId)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{mPart})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() {
		_ = abNet.Close()
	})
	return abNet
}

func startRPCServer(t *testing.T, partition *testpartition.NodePartition) (addr string) {
	// start rpc server for network.Nodes[0]
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer, err := initRPCServer(partition.Nodes[0].Node)
	require.NoError(t, err)

	t.Cleanup(func() {
		grpcServer.GracefulStop()
	})
	go func() {
		require.NoError(t, grpcServer.Serve(listener), "gRPC server exited with error")
	}()

	return listener.Addr().String()
}

func initRPCServer(node *partition.Node) (*grpc.Server, error) {
	rpcServer, err := rpc.NewGRPCServer(node)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer()
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
