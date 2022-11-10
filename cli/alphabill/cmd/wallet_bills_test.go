package cmd

import (
	"fmt"
	"path"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestWalletBillsListCmd_EmptyWallet(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout, err := execBillsCommand(homedir, "list")
	require.NoError(t, err)
	verifyStdout(t, stdout, "Wallet is empty.")
}

func TestWalletBillsListCmd(t *testing.T) {
	homedir, _ := setupInfra(t)

	// verify initial bill in list command
	stdout, err := execBillsCommand(homedir, "list")
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 10000")

	// send 3 txs to yourself
	address := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	for i := 1; i <= 3; i++ {
		stdout, _ = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s", i, address))
		verifyStdout(t, stdout, "Successfully confirmed transaction(s)")
	}

	// verify list bills shows all 4 bills
	stdout, err = execBillsCommand(homedir, "list")
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 9994")
	// remining 3 bills are in sorted by bill ids which can change because of undeterministic timeout value,
	// so we just check the length
	require.Len(t, stdout.lines, 4)
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
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath))

	// verify export with --bill-id flag
	stdout, err = execBillsCommand(homedir, "export --bill-id 0000000000000000000000000000000000000000000000000000000000000001 --output-path "+homedir)
	require.NoError(t, err)
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath))

	// verify export with no flags outputs all bills
	stdout, err = execBillsCommand(homedir, "export --output-path "+homedir)
	require.NoError(t, err)
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
	stdout, err = execBillsCommand(homedir, fmt.Sprintf("import"))
	require.ErrorContains(t, err, "required flag(s) \"bill-file\", \"trust-base-file\" not set")

	// test invalid block proof cannot be imported
	billsFile, _ := util.ReadJsonFile(billsFilePath, &BillsDTO{})
	billsFile.Bills[0].Proof.BlockHeaderHash = make([]byte, 32)
	invalidBillsFilePath := path.Join(homedir, "invalid-bills.json")
	_ = util.WriteJsonFile(invalidBillsFilePath, billsFile)

	stdout, err = execBillsCommand(homedir, fmt.Sprintf("import --bill-file=%s --trust-base-file=%s", invalidBillsFilePath, trustBaseFilePath))
	require.ErrorContains(t, err, "proof verification failed")

	// TODO send money between wallets and verify export and import (requires split block proof fix)
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
	pubkey := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	pubkeyBytes, _ := hexutil.Decode(pubkey)
	transferInitialBillTx, _ := createInitialBillTransferTx(pubkeyBytes, initialBill.ID, initialBill.Value, 10000)
	err := network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// create wallet
	homedir := createNewTestWallet(t)

	// sync wallet
	waitForBalance(t, "wallet-test", initialBill.Value, 0)

	return homedir, network
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
