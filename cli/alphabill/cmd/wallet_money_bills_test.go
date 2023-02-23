package cmd

import (
	"fmt"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
	"path"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
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
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{billId: uint256.NewInt(1), billValue: 1})
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
		billsList = billsList + fmt.Sprintf(`{"id":"%s","value":%d,"txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},`, toBillId(uint256.NewInt(uint64(i))), i)
	}
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: fmt.Sprintf(`{"total": 4, "bills": [%s]}`, strings.TrimSuffix(billsList, ","))})
	defer mockServer.Close()

	// verify list bills shows all 4 bills
	stdout, err := execBillsCommand(homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 1")
	verifyStdout(t, stdout, "#2 0x0000000000000000000000000000000000000000000000000000000000000002 2")
	verifyStdout(t, stdout, "#3 0x0000000000000000000000000000000000000000000000000000000000000003 3")
	verifyStdout(t, stdout, "#4 0x0000000000000000000000000000000000000000000000000000000000000004 4")
	require.Len(t, stdout.lines, 5)
}

func TestWalletBillsListCmd_ExtraAccount(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{billId: uint256.NewInt(1), billValue: 1})
	defer mockServer.Close()

	// add new key
	stdout, err := execCommand(homedir, "add-key")
	require.NoError(t, err)

	// verify list bills for specific account only shows given account bills
	stdout, err = execBillsCommand(homedir, "list -k 2 --alphabill-api-uri "+addr.Host)
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

	mockServer, addr := mockBackendCalls(&backendMockReturnConf{billId: uint256.NewInt(1), billValue: 1, customFullPath: "/" + client.ListBillsPath + "?pubkey=" + pubKey2, customResponse: `{"total": 0, "bills": []}`})
	defer mockServer.Close()

	// verify both accounts are listed
	stdout, err = execBillsCommand(homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 1")
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
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customPath: "/" + client.ProofPath, customResponse: fmt.Sprintf(`{"bills": [{"id":"%s","value":%d,"txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","is_dc_bill":false}]}`, toBillId(uint256.NewInt(uint64(1))), 1)})
	defer mockServer.Close()

	// verify export with --bill-id flag
	billFilePath := path.Join(homedir, "bill-0x0000000000000000000000000000000000000000000000000000000000000001.json")
	stdout, err := execBillsCommand(homedir, "export --bill-id 0000000000000000000000000000000000000000000000000000000000000001 --output-path "+homedir+" --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath))
}

func TestWalletBillsExportCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	billsList := ""
	for i := 1; i <= 4; i++ {
		billsList = billsList + fmt.Sprintf(`{"id":"%s","value":%d,"txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},`, toBillId(uint256.NewInt(uint64(i))), i)
	}
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: fmt.Sprintf(`{"total": 4, "bills": [%s]}`, strings.TrimSuffix(billsList, ",")), customPath: "/" + client.ProofPath, customResponse: fmt.Sprintf(`{"bills": [{"id":"%s","value":%d,"txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","is_dc_bill":false}]}`, toBillId(uint256.NewInt(uint64(1))), 1)})
	defer mockServer.Close()

	// verify export with no flags outputs all bills
	billFilePath := path.Join(homedir, "bills.json")
	stdout, err := execBillsCommand(homedir, "export --output-path "+homedir+" --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath))
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
