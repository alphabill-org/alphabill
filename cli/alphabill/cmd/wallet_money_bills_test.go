package cmd

import (
	"crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
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
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 0.000'000'01")
	verifyStdout(t, stdout, "#2 0x0000000000000000000000000000000000000000000000000000000000000002 0.000'000'02")
	verifyStdout(t, stdout, "#3 0x0000000000000000000000000000000000000000000000000000000000000003 0.000'000'03")
	verifyStdout(t, stdout, "#4 0x0000000000000000000000000000000000000000000000000000000000000004 0.000'000'04")
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

	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		billId:         uint256.NewInt(1),
		billValue:      1e9,
		customFullPath: "/" + client.ListBillsPath + "?pubkey=" + pubKey2 + "&includedcbills=false&includedcmetadata=false",
		customResponse: `{"total": 0, "bills": []}`})
	defer mockServer.Close()

	// verify both accounts are listed
	stdout, err = execBillsCommand(homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 10")
	verifyStdout(t, stdout, "Account #2 - empty")
}

func TestWalletBillsListCmd_ShowUnswappedFlag(t *testing.T) {
	homedir := createNewTestWallet(t)

	// get pub key
	stdout, err := execCommand(homedir, "get-pubkeys")
	require.NoError(t, err)
	pubKey := strings.Split(stdout.lines[0], " ")[1]

	// verify no -s flag sends includedcbills=false by default
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		customFullPath: "/" + client.ListBillsPath + "?pubkey=" + pubKey + "&includedcbills=false&includedcmetadata=false",
		customResponse: `{"total": 1, "bills": [{"value":"22222222"}]}`})

	stdout, err = execBillsCommand(homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x 0.222'222'22")
	mockServer.Close()

	// verify -s flag sends includedcbills=true
	mockServer, addr = mockBackendCalls(&backendMockReturnConf{
		customFullPath: "/" + client.ListBillsPath + "?pubkey=" + pubKey + "&includedcbills=true&includedcmetadata=false",
		customResponse: `{"total": 1, "bills": [{"value":"33333333"}]}`})

	stdout, err = execBillsCommand(homedir, "list --alphabill-api-uri "+addr.Host+" -s")
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x 0.333'333'33")
	mockServer.Close()
}

func TestWalletBillsExportCmd_Error(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{})
	defer mockServer.Close()

	// verify exporting non-existent bill returns error
	_, err := execBillsCommand(homedir, "export --bill-id=00 --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "proof not found for bill 0x00")
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

func TestWalletBillsExportCmd_ShowUnswappedFlag(t *testing.T) {
	homedir := createNewTestWallet(t)

	// get pub key
	stdout, err := execCommand(homedir, "get-pubkeys")
	require.NoError(t, err)
	pubKey := strings.Split(stdout.lines[0], " ")[1]

	// verify no -s flag sends includedcbills=false by default
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		proofList:      `{"bills": [{"id":"` + toBillId(uint256.NewInt(uint64(2))) + `","value":"22222222"}]}`,
		customFullPath: "/" + client.ListBillsPath + "?pubkey=" + pubKey + "&includedcbills=false&includedcmetadata=false",
		customResponse: `{"total": 1, "bills": [{"id":"` + toBillId(uint256.NewInt(uint64(2))) + `","value":"22222222"}]}`})

	stdout, err = execBillsCommand(homedir, "export --output-path "+homedir+" --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	billFilePath2 := filepath.Join(homedir, "bill-0x0000000000000000000000000000000000000000000000000000000000000002.json")
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath2))
	mockServer.Close()

	// verify -s flag sends includedcbills=true
	mockServer, addr = mockBackendCalls(&backendMockReturnConf{
		proofList:      `{"bills": [{"id":"` + toBillId(uint256.NewInt(uint64(3))) + `","value":"33333333"}]}`,
		customFullPath: "/" + client.ListBillsPath + "?pubkey=" + pubKey + "&includedcbills=true&includedcmetadata=false",
		customResponse: `{"total": 1, "bills": [{"id":"` + toBillId(uint256.NewInt(uint64(3))) + `","value":"33333333"}]}`})

	stdout, err = execBillsCommand(homedir, "export --output-path "+homedir+" --alphabill-api-uri "+addr.Host+" -s")
	require.NoError(t, err)
	billFilePath3 := filepath.Join(homedir, "bill-0x0000000000000000000000000000000000000000000000000000000000000003.json")
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", billFilePath3))
	mockServer.Close()
}

func spendInitialBillWithFeeCredits(t *testing.T, abNet *testpartition.AlphabillNetwork, initialBill *money.InitialBill, pubkey string) uint64 {
	pubkeyBytes, _ := hexutil.Decode(pubkey)
	pubkeyHash := hash.Sum256(pubkeyBytes)
	absoluteTimeout := uint64(10000)

	txFee := uint64(1)
	feeAmount := uint64(2)
	fcrID := utiltx.SameShardIDBytes(initialBill.ID, pubkeyHash)
	unitID := util.Uint256ToBytes(initialBill.ID)
	moneyPart, err := abNet.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)

	// create transferFC
	transferFC, err := createTransferFC(feeAmount, unitID, fcrID, 0, absoluteTimeout)
	require.NoError(t, err)

	// send transferFC
	err = moneyPart.SubmitTx(transferFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPart, transferFC), test.WaitDuration, test.WaitTick)
	_, transferFCProof, transferFCRecord, err := moneyPart.GetTxProof(transferFC)
	require.NoError(t, err)

	// verify proof
	err = types.VerifyTxProof(transferFCProof, transferFCRecord, abNet.RootPartition.TrustBase, crypto.SHA256)
	require.NoError(t, err)

	// create addFC
	addFC, err := createAddFC(fcrID, script.PredicateAlwaysTrue(), transferFCRecord, transferFCProof, absoluteTimeout, feeAmount)
	require.NoError(t, err)

	// send addFC
	err = moneyPart.SubmitTx(addFC)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPart, addFC), test.WaitDuration, test.WaitTick)

	// create transfer tx
	remainingValue := initialBill.Value - feeAmount - txFee
	tx, err := createTransferTx(pubkeyBytes, unitID, remainingValue, fcrID, absoluteTimeout, transferFC.Hash(crypto.SHA256))
	require.NoError(t, err)

	// send transfer tx
	err = moneyPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPart, tx), test.WaitDuration, test.WaitTick)

	return remainingValue
}

func createTransferTx(pubKey []byte, billID []byte, billValue uint64, fcrID []byte, timeout uint64, backlink []byte) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: billValue,
		Backlink:    backlink,
	}
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			UnitID:     billID,
			Type:       money.PayloadTypeTransfer,
			SystemID:   []byte{0, 0, 0, 0},
			Attributes: attrBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: 1,
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
	}
	return tx, nil
}

func createTransferFC(feeAmount uint64, unitID []byte, targetUnitID []byte, t1, t2 uint64) (*types.TransactionOrder, error) {
	attr := &transactions.TransferFeeCreditAttributes{
		Amount:                 feeAmount,
		TargetSystemIdentifier: []byte{0, 0, 0, 0},
		TargetRecordID:         targetUnitID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
	}
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   []byte{0, 0, 0, 0},
			Type:       transactions.PayloadTypeTransferFeeCredit,
			UnitID:     unitID,
			Attributes: attrBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           t2,
				MaxTransactionFee: 1,
			},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
	}
	return tx, nil
}

func createAddFC(unitID []byte, ownerCondition []byte, transferFC *types.TransactionRecord, transferFCProof *types.TxProof, timeout uint64, maxFee uint64) (*types.TransactionOrder, error) {
	attr := &transactions.AddFeeCreditAttributes{
		FeeCreditTransfer:       transferFC,
		FeeCreditTransferProof:  transferFCProof,
		FeeCreditOwnerCondition: ownerCondition,
	}
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   []byte{0, 0, 0, 0},
			Type:       transactions.PayloadTypeAddFeeCredit,
			UnitID:     unitID,
			Attributes: attrBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: maxFee,
			},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
	}
	return tx, nil
}

func execBillsCommand(homeDir, command string) (*testConsoleWriter, error) {
	return execCommand(homeDir, " bills "+command)
}
