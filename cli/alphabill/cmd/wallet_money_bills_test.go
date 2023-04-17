package cmd

import (
	"crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
)

func TestWalletBillsListCmd_EmptyWallet(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: `{"total": 0, "bills": []}`})
	defer mockServer.Close()
	stdout, err := execBillsCommand(homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1 - empty")
}

func TestWalletBillsListCmd_Single(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{billId: uint256.NewInt(1), billValue: 1e8})
	defer mockServer.Close()

	// verify bill in list command
	stdout, err := execBillsCommand(homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 1")
}

func TestWalletBillsListCmd_Multiple(t *testing.T) {
	homedir := createNewTestWallet(t)

	billsList := ""
	for i := 1; i <= 4; i++ {
		billsList = billsList + fmt.Sprintf(`{"id":"%s","value":"%d","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},`, toBillId(uint256.NewInt(uint64(i))), i)
	}
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: fmt.Sprintf(`{"total": 4, "bills": [%s]}`, strings.TrimSuffix(billsList, ","))})
	defer mockServer.Close()

	// verify list bills shows all 4 bills
	stdout, err := execBillsCommand(homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 0.00000001")
	verifyStdout(t, stdout, "#2 0x0000000000000000000000000000000000000000000000000000000000000002 0.00000002")
	verifyStdout(t, stdout, "#3 0x0000000000000000000000000000000000000000000000000000000000000003 0.00000003")
	verifyStdout(t, stdout, "#4 0x0000000000000000000000000000000000000000000000000000000000000004 0.00000004")
	require.Len(t, stdout.lines, 5)
}

func TestWalletBillsListCmd_ExtraAccount(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{billId: uint256.NewInt(1), billValue: 1})
	defer mockServer.Close()

	// add new key
	_, err := execCommand(homedir, "add-key")
	require.NoError(t, err)

	// verify list bills for specific account only shows given account bills
	stdout, err := execBillsCommand(homedir, "list -k 2 --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	lines := stdout.lines
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "Account #2")
	require.Contains(t, lines[1], "#1")
}

func TestWalletBillsListCmd_ExtraAccountTotal(t *testing.T) {
	homedir := createNewTestWallet(t)

	// add new key
	stdout, err := execCommand(homedir, "add-key")
	require.NoError(t, err)
	pubKey2 := strings.Split(stdout.lines[0], " ")[3]

	mockServer, addr := mockBackendCalls(&backendMockReturnConf{billId: uint256.NewInt(1), billValue: 1e9, customFullPath: "/" + client.ListBillsPath + "?pubkey=" + pubKey2, customResponse: `{"total": 0, "bills": []}`})
	defer mockServer.Close()

	// verify both accounts are listed
	stdout, err = execBillsCommand(homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 10")
	verifyStdout(t, stdout, "Account #2 - empty")
}

func TestWalletBillsExportCmd_Error(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{})
	defer mockServer.Close()

	// verify exporting non-existent bill returns error
	_, err := execBillsCommand(homedir, "export --bill-id=00 --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "bill does not exist")
}

func TestWalletBillsExportCmd_BillIdFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customPath: "/" + client.ProofPath, customResponse: fmt.Sprintf(`{"bills": [{"id":"%s","value":"%d","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","is_dc_bill":false}]}`, toBillId(uint256.NewInt(uint64(1))), 1)})
	defer mockServer.Close()

	// verify export with --bill-id flag
	billFilePath := filepath.Join(homedir, "bill-0x0000000000000000000000000000000000000000000000000000000000000001.json")
	stdout, err := execBillsCommand(homedir, "export --bill-id 0000000000000000000000000000000000000000000000000000000000000001 --output-path "+homedir+" --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	require.Len(t, stdout.lines, 1)
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath))
}

func TestWalletBillsExportCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	billsList := ""
	for i := 1; i <= 4; i++ {
		billsList = billsList + fmt.Sprintf(`{"id":"%s","value":"%d","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},`, toBillId(uint256.NewInt(uint64(i))), i)
	}
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: fmt.Sprintf(`{"total": 4, "bills": [%s]}`, strings.TrimSuffix(billsList, ",")), customPath: "/" + client.ProofPath, customResponse: fmt.Sprintf(`{"bills": [{"id":"%s","value":"%d","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","is_dc_bill":false}]}`, toBillId(uint256.NewInt(uint64(1))), 1)})
	defer mockServer.Close()

	// verify export with no flags outputs all bills
	billFilePath := filepath.Join(homedir, "bills.json")
	stdout, err := execBillsCommand(homedir, "export --output-path "+homedir+" --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	require.Len(t, stdout.lines, 1)
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath))
}

// setupInfra starts money partiton, sends initial bill to wallet, syncs wallet.
// Returns home dir of wallet and alphabill partition.
func setupInfra(t *testing.T) (string, *testpartition.AlphabillPartition) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill, true)
	startRPCServer(t, network, ":9543")

	// transfer initial bill to wallet pubkey
	spendInitialBillWithFeeCredits(t, network, initialBill)

	// create wallet
	homedir := createNewTestWallet(t)

	// sync wallet
	waitForBalance(t, homedir, initialBill.Value-3, 0) // initial bill minus txfees

	return homedir, network
}

func spendInitialBillWithFeeCredits(t *testing.T, network *testpartition.AlphabillPartition, initialBill *moneytx.InitialBill) uint64 {
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

	// verify proof
	gtx, err := moneytx.NewMoneyTx([]byte{0, 0, 0, 0}, transferFC)
	require.NoError(t, err)
	require.NoError(t, transferFCProof.Verify(unitID, gtx, network.TrustBase, crypto.SHA256))

	// create addFC
	addFC, err := createAddFC(fcrID, script.PredicateAlwaysTrue(), transferFC, transferFCProof, absoluteTimeout, feeAmount)
	require.NoError(t, err)
	// send addFC
	err = network.SubmitTx(addFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(addFC, network), test.WaitDuration, test.WaitTick)

	// create transfer tx
	transferFCWrapper, err := transactions.NewFeeCreditTx(transferFC)
	require.NoError(t, err)
	remainingValue := initialBill.Value - feeAmount - txFee
	tx, err := createTransferTx(pubkeyBytes, unitID, remainingValue, fcrID, absoluteTimeout, transferFCWrapper.Hash(crypto.SHA256))
	require.NoError(t, err)

	// send transfer tx
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	return remainingValue
}

func spendInitialBill(t *testing.T, network *testpartition.AlphabillPartition, initialBill *moneytx.InitialBill) uint64 {
	pubkey := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	pubkeyBytes, _ := hexutil.Decode(pubkey)
	absoluteTimeout := uint64(10000)

	tx, err := createTransferTx(pubkeyBytes, util.Uint256ToBytes(initialBill.ID), initialBill.Value, nil, absoluteTimeout, nil)
	require.NoError(t, err)

	// send transfer tx
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	return initialBill.Value
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
	err := anypb.MarshalFrom(tx.TransactionAttributes, &moneytx.TransferAttributes{
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
			MaxFee:  fc.FixedFee(1)(),
		},
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &transactions.TransferFeeCreditAttributes{
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
	err := anypb.MarshalFrom(tx.TransactionAttributes, &transactions.AddFeeCreditAttributes{
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
