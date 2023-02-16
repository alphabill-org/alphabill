package cmd

import (
	"crypto"
	"fmt"
	"path"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestWalletBillsListCmd_EmptyWallet(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout, err := execBillsCommand(homedir, "list")
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1 - empty")
}

func TestWalletBillsListCmd(t *testing.T) {
	homedir, _ := setupInfra(t)
	val := 9997 // initial bill minus fees

	// verify initial bill in list command
	stdout, err := execBillsCommand(homedir, "list")
	require.NoError(t, err)
	verifyStdout(t, stdout, fmt.Sprintf("#1 0x0000000000000000000000000000000000000000000000000000000000000001 %d", val))

	// create fee credit for txs
	feeSum := 10
	stdout, _ = execCommand(homedir, fmt.Sprintf("fees add --amount %d", feeSum))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", feeSum))
	val -= feeSum // 9987
	val -= 1      // 9986 transferFC tx fee

	// send 3 txs to yourself
	address := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	for i := 1; i <= 3; i++ {
		stdout, err = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s", i, address))
		require.NoError(t, err)
		verifyStdout(t, stdout, "Successfully confirmed transaction(s)")
		val -= i
	}

	// verify list bills shows all 4 bills
	stdout, err = execBillsCommand(homedir, "list")
	require.NoError(t, err)

	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, fmt.Sprintf("#1 0x0000000000000000000000000000000000000000000000000000000000000001 %d", val)) // val=9980
	// remining 3 bills are in sorted by bill ids which can change because of undeterministic timeout value,
	// so we just check the length
	require.Len(t, stdout.lines, 5)

	// add new key and send transactions to it
	address2 := "0x02d36c574db299904b285aaeb57eb7b1fa145c43af90bec3c635c4174c224587b6"
	_, _ = execCommand(homedir, "add-key")
	_, _ = execCommand(homedir, fmt.Sprintf("send -k 1 --amount %d --address %s", 1, address2))
	_, _ = execCommand(homedir, fmt.Sprintf("send -k 1 --amount %d --address %s", 2, address2))
	_, _ = execCommand(homedir, fmt.Sprintf("send -k 1 --amount %d --address %s", 3, address2))
	_, _ = execCommand(homedir, "sync -u localhost:9543")

	// verify list bills for specfic account only shows given account bills
	stdout, err = execBillsCommand(homedir, "list -k 2")
	require.NoError(t, err)
	lines := stdout.lines
	require.Len(t, lines, 4)
	require.Contains(t, lines[0], "Account #2")
	require.Contains(t, lines[1], "#1")
	require.Contains(t, lines[2], "#2")
	require.Contains(t, lines[3], "#3")
}

func TestWalletBillsExportCmd(t *testing.T) {
	homedir, _ := setupInfra(t)

	// verify exporting non-existent bill returns error
	_, err := execBillsCommand(homedir, "export --bill-id=00")
	require.ErrorContains(t, err, "bill does not exist")

	// verify export with --bill-order-number flag
	billFilePath := path.Join(homedir, "bill-0x0000000000000000000000000000000000000000000000000000000000000001.json")
	stdout, err := execBillsCommand(homedir, "export --bill-order-number 1 --output-path "+homedir)
	require.NoError(t, err)
	require.Len(t, stdout.lines, 1)
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath))

	// verify export with --bill-id flag
	stdout, err = execBillsCommand(homedir, "export --bill-id 0000000000000000000000000000000000000000000000000000000000000001 --output-path "+homedir)
	require.NoError(t, err)
	require.Len(t, stdout.lines, 1)
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath))

	// verify export with no flags outputs all bills
	stdout, err = execBillsCommand(homedir, "export --output-path "+homedir)
	require.NoError(t, err)
	require.Len(t, stdout.lines, 1)
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath))
}

func TestWalletBillsImportCmd(t *testing.T) {
	homedir, network := setupInfra(t)
	billsFilePath := path.Join(homedir, "bill-0x0000000000000000000000000000000000000000000000000000000000000001.json")
	trustBaseFilePath := path.Join(homedir, "trust-base.json")
	_ = createTrustBaseFile(trustBaseFilePath, network)

	// export the initial bill
	stdout, err := execBillsCommand(homedir, "export --output-path "+homedir)
	require.NoError(t, err)
	require.Contains(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billsFilePath))

	// import the same bill exported in previous step
	stdout, err = execBillsCommand(homedir, fmt.Sprintf("import --bill-file=%s --trust-base-file=%s", billsFilePath, trustBaseFilePath))
	require.NoError(t, err)
	require.Contains(t, stdout.lines[0], "Successfully imported bill(s).")

	// test import required flags
	_, err = execBillsCommand(homedir, "import")
	require.ErrorContains(t, err, "required flag(s) \"bill-file\", \"trust-base-file\" not set")

	// test invalid block proof cannot be imported
	billsFile, _ := moneytx.ReadBillsFile(billsFilePath)
	billsFile.Bills[0].TxProof.Proof.BlockHeaderHash = make([]byte, 32)
	invalidBillsFilePath := path.Join(homedir, "invalid-bills.json")
	_ = moneytx.WriteBillsFile(invalidBillsFilePath, billsFile)

	_, err = execBillsCommand(homedir, fmt.Sprintf("import --bill-file=%s --trust-base-file=%s", invalidBillsFilePath, trustBaseFilePath))
	require.ErrorContains(t, err, "proof verification failed")

	// create fees for txs
	stdout, _ = execCommand(homedir, fmt.Sprintf("fees add --amount %d", 10))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", 10))

	// test that proof from "send --output-path" can be imported
	// send a bill to account number 2
	pubKeyAcc2 := "0x02d36c574db299904b285aaeb57eb7b1fa145c43af90bec3c635c4174c224587b6"
	stdout = execWalletCmd(t, homedir, fmt.Sprintf("send --amount 1 --address %s --output-path %s", pubKeyAcc2, homedir))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], "Transaction proof(s) saved to: ")
	outputFile := stdout.lines[1][len("Transaction proof(s) saved to: "):]

	// add account number 2 to wallet
	stdout = execWalletCmd(t, homedir, "add-key")
	require.Contains(t, stdout.lines[0], fmt.Sprintf("Added key #2 %s", pubKeyAcc2))

	// import bill to account number 2
	stdout, err = execBillsCommand(homedir, fmt.Sprintf("import --key 2 --bill-file=%s --trust-base-file=%s", outputFile, trustBaseFilePath))
	require.NoError(t, err)
	require.Contains(t, stdout.lines[0], "Successfully imported bill(s).")

	// add account number 3 to wallet
	pubKeyAcc3 := "0x02f6cbeacfd97ebc9b657081eb8b6c9ed3a588646d618ddbd03e198290af94c9d2"
	stdout = execWalletCmd(t, homedir, "add-key")
	require.Contains(t, stdout.lines[0], fmt.Sprintf("Added key #3 %s", pubKeyAcc3))

	// verify that the same bill cannot be imported to account number 3
	_, err = execBillsCommand(homedir, fmt.Sprintf("import --key 3 --bill-file=%s --trust-base-file=%s", outputFile, trustBaseFilePath))
	require.ErrorContains(t, err, "invalid bearer predicate")
}

// setupInfra starts money partiton, sends initial bill to wallet, syncs wallet.
// Returns home dir of wallet and alphabill partition.
func setupInfra(t *testing.T) (string, *testpartition.AlphabillPartition) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

	// transfer initial bill to wallet pubkey
	spendInitialBill(t, network, initialBill)

	// create wallet
	homedir := createNewTestWallet(t)

	// sync wallet
	waitForBalance(t, homedir, initialBill.Value-3, 0) // initial bill minus txfees

	return homedir, network
}

func spendInitialBill(t *testing.T, network *testpartition.AlphabillPartition, initialBill *moneytx.InitialBill) {
	pubkey := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	pubkeyBytes, _ := hexutil.Decode(pubkey)
	pubkeyHash := hash.Sum256(pubkeyBytes)
	absoluteTimeout := uint64(10000)

	txFee := uint64(1)
	feeAmount := uint64(2)
	fcrID := utiltx.SameShardIDBytes(initialBill.ID, pubkeyHash)
	unitID := util.Uint256ToBytes(initialBill.ID)

	// create transferFC
	transferFC, err := createTransferFC(feeAmount, unitID, fcrID, 0, absoluteTimeout)
	require.NoError(t, err)
	// send transferFC
	err = network.SubmitTx(transferFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferFC, network), test.WaitDuration, test.WaitTick)
	transferFCProof := getBlockProof(t, transferFC, network)

	// create addFC
	addFC, err := createAddFC(fcrID, script.PredicateAlwaysTrue(), transferFC, transferFCProof, absoluteTimeout, feeAmount)
	require.NoError(t, err)
	// send addFC
	err = network.SubmitTx(addFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(addFC, network), test.WaitDuration, test.WaitTick)

	// create transfer tx
	transferFCWrapper, err := fc.NewFeeCreditTx(transferFC)
	require.NoError(t, err)
	tx, err := createTransferTx(pubkeyBytes, unitID, initialBill.Value-feeAmount-txFee, fcrID, absoluteTimeout, transferFCWrapper.Hash(crypto.SHA256))
	require.NoError(t, err)
	// send transfer tx
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}

func createTransferTx(pubKey []byte, billId []byte, billValue uint64, fcrID []byte, timeout uint64, backlink []byte) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:                billId,
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: new(anypb.Any),
		OwnerProof:            script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout:           timeout,
			MaxFee:            1,
			FeeCreditRecordId: fcrID,
		},
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &moneytx.TransferOrder{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: billValue,
		Backlink:    backlink,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createTransferFC(feeAmount uint64, unitID []byte, targetUnitID []byte, t1, t2 uint64) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:                unitID,
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: new(anypb.Any),
		OwnerProof:            script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout: t2,
			MaxFee:  1,
		},
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &fc.TransferFeeCreditOrder{
		Amount:                 feeAmount,
		TargetSystemIdentifier: []byte{0, 0, 0, 0},
		TargetRecordId:         targetUnitID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createAddFC(unitID []byte, ownerCondition []byte, transferFC *txsystem.Transaction, transferFCProof *block.BlockProof, timeout uint64, maxFee uint64) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:                unitID,
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: new(anypb.Any),
		OwnerProof:            script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout: timeout,
			MaxFee:  maxFee,
		},
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &fc.AddFeeCreditOrder{
		FeeCreditTransfer:       transferFC,
		FeeCreditTransferProof:  transferFCProof,
		FeeCreditOwnerCondition: ownerCondition,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func getBlockProof(t *testing.T, tx *txsystem.Transaction, network *testpartition.AlphabillPartition) *block.BlockProof {
	// create adapter for conversion interface
	txConverter := func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
		return moneytx.NewMoneyTx([]byte{0, 0, 0, 0}, tx)
	}
	_, ttt, err := network.GetBlockProof(tx, txConverter)
	require.NoError(t, err)
	return ttt
}

// createTrustBaseFile extracts and saves trust-base file from testpartition.AlphabillPartition
func createTrustBaseFile(filePath string, network *testpartition.AlphabillPartition) error {
	tb := &TrustBase{RootValidators: []*genesis.PublicKeyInfo{}}
	for k, v := range network.TrustBase {
		pk, _ := v.MarshalPublicKey()
		tb.RootValidators = append(tb.RootValidators, &genesis.PublicKeyInfo{
			NodeIdentifier:   k,
			SigningPublicKey: pk,
		})
	}
	return util.WriteJsonFile(filePath, tb)
}

func execBillsCommand(homeDir, command string) (*testConsoleWriter, error) {
	return execCommand(homeDir, " bills "+command)
}
