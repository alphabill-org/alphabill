package money

const port = 9111

/*
func TestCollectDustTimeoutReached(t *testing.T) {
	// start server
	serverService := testserver.NewTestAlphabillServiceServer()
	server := testserver.StartServer(port, serverService)
	t.Cleanup(server.GracefulStop)

	// setup wallet
	_ = log.InitStdoutLogger(log.DEBUG)
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, "")
	require.NoError(t, err)
	bills := []*Bill{addBill(100), addBill(200)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount)
	restServer, serverAddr := mockBackendCalls(&backendMockReturnConf{balance: 300, customBillList: billsList, proofList: proofList})
	restClient, err := testclient.NewClient(serverAddr.Host)
	w, err := LoadExistingWallet(&WalletConfig{AlphabillClientConfig: client.AlphabillClientConfig{Uri: fmt.Sprintf(":%d", port)}}, am, restClient)
	require.NoError(t, err)

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
	require.Len(t, serverService.GetProcessedTransactions(), 2)
	require.NoError(t, err)

	// and dc wg metadata is saved
	require.Len(t, w.dcWg.swaps, 1)
	dcNonce := calculateDcNonce(bills)
	require.EqualValues(t, w.dcWg.swaps[*uint256.NewInt(0).SetBytes(dcNonce)], dcTimeoutBlockCount)

	for blockNo := uint64(1); blockNo <= dcTimeoutBlockCount; blockNo++ {
		b := &block.Block{
			SystemIdentifier:   alphabillMoneySystemId,
			BlockNumber:        blockNo,
			PreviousBlockHash:  hash.Sum256([]byte{}),
			Transactions:       []*txsystem.Transaction{},
			UnicityCertificate: &certificates.UnicityCertificate{},
		}
		serverService.SetBlock(blockNo, b)
	}
	// when dc timeout is reached
	serverService.SetMaxBlockNumber(dcTimeoutBlockCount)

	restServer.Close()
	k, _ := am.GetAccountKey(0)
	dcBills := []*Bill{addDcBill(t, k, uint256.NewInt(1), 100, dcTimeoutBlockCount), addDcBill(t, k, uint256.NewInt(1), 200, dcTimeoutBlockCount)}
	dcBillsList := createBillListJsonResponse(dcBills)
	dcProofList := createBlockProofJsonResponse(t, dcBills, nil, 0, dcTimeoutBlockCount)
	_, serverAddr = mockBackendCalls(&backendMockReturnConf{balance: 300, customBillList: dcBillsList, proofList: dcProofList})
	restClient, err = testclient.NewClient(serverAddr.Host)
	w, _ = LoadExistingWallet(&WalletConfig{AlphabillClientConfig: client.AlphabillClientConfig{Uri: fmt.Sprintf(":%d", port)}}, am, restClient)

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
/*func TestCollectDustInMultiAccountWallet(t *testing.T) {
	// start network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	addr := "localhost:9544"
	startRPCServer(t, network, addr)
	restAddr := startRestServer(t, addr)

	// setup wallet with multiple keys
	_ = log.InitStdoutLogger(log.DEBUG)
	_ = DeleteWalletDbs(os.TempDir())
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	err = CreateNewWallet(am, "")
	require.NoError(t, err)
	restClient, err := testclient.NewClient(restAddr)
	w, err := LoadExistingWallet(&WalletConfig{AlphabillClientConfig: client.AlphabillClientConfig{Uri: addr}}, am, restClient)
	require.NoError(t, err)
	require.NoError(t, err)

	_, _, _ = am.AddAccount()
	_, _, _ = am.AddAccount()

	// transfer initial bill to wallet 1
	pubkeys, err := am.GetPublicKeys()
	require.NoError(t, err)

	transferInitialBillTx, err := createInitialBillTransferTx(pubkeys[0], initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
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
		err := w.CollectDust(ctx)
		if err == nil {
			defer cancel() // signal Sync to cancel
		}
		return err
	})

	// wait for dust collection to finish
	err = group.Wait()
	require.NoError(t, err)
}

func sendToAccount(t *testing.T, w *Wallet, accountIndexTo uint64) {
	receiverPubkey, err := w.am.GetPublicKey(accountIndexTo)
	require.NoError(t, err)

	prevBalance, err := w.GetBalance(GetBalanceCmd{AccountIndex: accountIndexTo})
	require.NoError(t, err)

	_, err = w.Send(context.Background(), SendCmd{ReceiverPubKey: receiverPubkey, Amount: 1})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_ = w.SyncToMaxBlockNumber(context.Background(), dcTimeoutBlockCount)
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

func startRestServer(t *testing.T, rpcAddr string) string {
	addr := "localhost:9545"
	server := money.NewHttpServer(addr, 100, createWalletBackend(t, rpcAddr))
	err := server.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})
	return addr
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

func createWalletBackend(t *testing.T, addr string) *money.WalletBackend {
	dbFile := filepath.Join(t.TempDir(), money.BoltBillStoreFileName)
	storage, _ := money.NewBoltBillStore(dbFile)
	bp := money.NewBlockProcessor(storage, backend.NewTxConverter([]byte{0, 0, 0, 0}))
	genericWallet := wallet.New().SetBlockProcessor(bp).SetABClient(client.New(client.AlphabillClientConfig{Uri: addr})).Build()
	return money.New(genericWallet, storage)
}*/
