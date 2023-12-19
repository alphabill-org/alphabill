package cmd

import (
	"crypto"
	"fmt"
	"strings"
	"testing"

	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wallet/money/backend/client"
)

func TestWalletBillsListCmd_EmptyWallet(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: `{"bills": []}`})
	defer mockServer.Close()
	stdout, err := execBillsCommand(testobserve.NewFactory(t), homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1 - empty")
}

func TestWalletBillsListCmd_Single(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{billID: money.NewBillID(nil, []byte{1}), billValue: 1e8})
	defer mockServer.Close()

	// verify bill in list command
	stdout, err := execBillsCommand(testobserve.NewFactory(t), homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00")
}

func TestWalletBillsListCmd_Multiple(t *testing.T) {
	homedir := createNewTestWallet(t)

	billsList := ""
	for i := 1; i <= 4; i++ {
		billsList = billsList + fmt.Sprintf(`{"id":"%s","value":"%d","txHash":"MHgwMzgwMDNlMjE4ZWVhMzYwY2JmNTgwZWJiOTBjYzhjOGNhZjBjY2VmNGJmNjYwZWE5YWI0ZmMwNmI1YzM2N2IwMzg=","isDCBill":false},`, toBase64(money.NewBillID(nil, []byte{byte(i)})), i)
	}
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: fmt.Sprintf(`{"bills": [%s]}`, strings.TrimSuffix(billsList, ","))})
	defer mockServer.Close()

	// verify list bills shows all 4 bills
	stdout, err := execBillsCommand(testobserve.NewFactory(t), homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 0.000'000'01")
	verifyStdout(t, stdout, "#2 0x000000000000000000000000000000000000000000000000000000000000000200 0.000'000'02")
	verifyStdout(t, stdout, "#3 0x000000000000000000000000000000000000000000000000000000000000000300 0.000'000'03")
	verifyStdout(t, stdout, "#4 0x000000000000000000000000000000000000000000000000000000000000000400 0.000'000'04")
	require.Len(t, stdout.lines, 5)
}

func TestWalletBillsListCmd_ExtraAccount(t *testing.T) {
	homedir := createNewTestWallet(t)
	logF := testobserve.NewFactory(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{billID: money.NewBillID(nil, []byte{1}), billValue: 1})
	defer mockServer.Close()

	// add new key
	_, err := execCommand(logF, homedir, "add-key")
	require.NoError(t, err)

	// verify list bills for specific account only shows given account bills
	stdout, err := execBillsCommand(logF, homedir, "list -k 2 --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	lines := stdout.lines
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "Account #2")
	require.Contains(t, lines[1], "#1")
}

func TestWalletBillsListCmd_ExtraAccountTotal(t *testing.T) {
	homedir := createNewTestWallet(t)
	logF := testobserve.NewFactory(t)

	// add new key
	stdout, err := execCommand(logF, homedir, "add-key")
	require.NoError(t, err)
	pubKey2 := strings.Split(stdout.lines[0], " ")[3]

	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		billID:         money.NewBillID(nil, []byte{1}),
		billValue:      1e9,
		customFullPath: "/" + client.ListBillsPath + "?includeDcBills=false&limit=100&pubkey=" + pubKey2,
		customResponse: `{"bills": []}`})
	defer mockServer.Close()

	// verify both accounts are listed
	stdout, err = execBillsCommand(logF, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "Account #1")
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 10")
	verifyStdout(t, stdout, "Account #2 - empty")
}

func TestWalletBillsListCmd_ShowUnswappedFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	logF := testobserve.NewFactory(t)

	// get pub key
	stdout, err := execCommand(logF, homedir, "get-pubkeys")
	require.NoError(t, err)
	pubKey := strings.Split(stdout.lines[0], " ")[1]

	// verify no -s flag sends includeDcBills=false by default
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		customFullPath: "/" + client.ListBillsPath + "?includeDcBills=false&limit=100&pubkey=" + pubKey,
		customResponse: `{"bills": [{"value":"22222222"}]}`})

	stdout, err = execBillsCommand(logF, homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x 0.222'222'22")
	mockServer.Close()

	// verify -s flag sends includeDcBills=true
	mockServer, addr = mockBackendCalls(&backendMockReturnConf{
		customFullPath: "/" + client.ListBillsPath + "?includeDcBills=true&limit=100&pubkey=" + pubKey,
		customResponse: `{"bills": [{"value":"33333333"}]}`})

	stdout, err = execBillsCommand(logF, homedir, "list --alphabill-api-uri "+addr.Host+" -s")
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x 0.333'333'33")
	mockServer.Close()
}

func TestWalletBillsListCmd_ShowLockedBills(t *testing.T) {
	homedir := createNewTestWallet(t)
	var billsList []string
	for i := 1; i <= 3; i++ {
		idBase64 := toBase64(money.NewBillID(nil, []byte{byte(i)}))
		billsList = append(billsList, fmt.Sprintf(`{"id":"%s","locked":"%d","value":"100000000"}`, idBase64, i))
	}
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{customBillList: fmt.Sprintf(`{"bills": [%s]}`, strings.Join(billsList, ","))})
	defer mockServer.Close()
	stdout, err := execBillsCommand(testobserve.NewFactory(t), homedir, "list --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00 (locked for adding fees)")
	verifyStdout(t, stdout, "#2 0x000000000000000000000000000000000000000000000000000000000000000200 1.000'000'00 (locked for reclaiming fees)")
	verifyStdout(t, stdout, "#3 0x000000000000000000000000000000000000000000000000000000000000000300 1.000'000'00 (locked for dust collection)")
}

func TestWalletBillsLockUnlockCmd_Ok(t *testing.T) {
	// create wallet
	am, homedir := createNewWallet(t)
	pubkey, _ := am.GetPublicKey(0)
	am.Close()

	// start money partition
	genesisConfig := &moneyGenesisConfig{
		InitialBillID:      defaultInitialBillID,
		InitialBillValue:   2e8,
		InitialBillOwner:   templates.NewP2pkh256BytesFromKey(pubkey),
		DCMoneySupplyValue: 10000,
	}
	moneyPartition := createMoneyPartition(t, genesisConfig, 1)
	logF := testobserve.NewFactory(t)
	_ = startAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	startPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	addr, _ := startMoneyBackend(t, moneyPartition, genesisConfig)

	// create fee credit for txs
	stdout, err := execCommand(logF, homedir, fmt.Sprintf("fees add --alphabill-api-uri %s", addr))
	require.NoError(t, err)

	// lock bill
	stdout, err = execBillsCommand(logF, homedir, fmt.Sprintf("lock --alphabill-api-uri %s --bill-id %s", addr, defaultInitialBillID))
	require.NoError(t, err)
	verifyStdout(t, stdout, "Bill locked successfully.")

	// verify bill locked
	stdout, err = execBillsCommand(logF, homedir, fmt.Sprintf("list --alphabill-api-uri %s", addr))
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00 (manually locked by user)")

	// unlock bill
	stdout, err = execBillsCommand(logF, homedir, fmt.Sprintf("unlock --alphabill-api-uri %s --bill-id %s", addr, defaultInitialBillID))
	require.NoError(t, err)
	verifyStdout(t, stdout, "Bill unlocked successfully.")

	// verify bill unlocked
	stdout, err = execBillsCommand(logF, homedir, fmt.Sprintf("list --alphabill-api-uri %s", addr))
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x000000000000000000000000000000000000000000000000000000000000000100 1.000'000'00")
}

func spendInitialBillWithFeeCredits(t *testing.T, abNet *testpartition.AlphabillNetwork, initialBillValue uint64, pk []byte) uint64 {
	absoluteTimeout := uint64(10000)
	initialBillID := defaultInitialBillID

	txFee := uint64(1)
	feeAmount := uint64(2)
	unitID := initialBillID
	moneyPart, err := abNet.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)

	// create transferFC
	transferFC, err := createTransferFC(feeAmount+txFee, unitID, fcrID, 0, absoluteTimeout)
	require.NoError(t, err)

	// send transferFC
	require.NoError(t, moneyPart.SubmitTx(transferFC))
	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPart, transferFC)
	require.NoError(t, err, "transfer fee credit tx failed")
	// verify proof
	require.NoError(t, types.VerifyTxProof(transferFCProof, transferFCRecord, abNet.RootPartition.TrustBase, crypto.SHA256))
	unitState, err := testpartition.WaitUnitProof(t, moneyPart, initialBillID, transferFC)
	require.NoError(t, err)
	ucValidator, err := abNet.GetValidator(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitState.Proof, crypto.SHA256, unitState.UnitData, ucValidator))
	var bill money.BillData
	require.NoError(t, unitState.UnmarshalUnitData(&bill))
	require.EqualValues(t, initialBillValue-txFee-feeAmount, bill.V)
	// create addFC
	addFC, err := createAddFC(fcrID, templates.AlwaysTrueBytes(), transferFCRecord, transferFCProof, absoluteTimeout, feeAmount)
	require.NoError(t, err)

	// send addFC
	require.NoError(t, moneyPart.SubmitTx(addFC))
	_, _, err = testpartition.WaitTxProof(t, moneyPart, addFC)
	require.NoError(t, err, "add fee credit tx failed")

	// create transfer tx
	remainingValue := initialBillValue - feeAmount - txFee
	tx, err := createTransferTx(pk, unitID, remainingValue, fcrID, absoluteTimeout, transferFCRecord.TransactionOrder.Hash(crypto.SHA256))
	require.NoError(t, err)

	// send transfer tx
	require.NoError(t, moneyPart.SubmitTx(tx))
	_, _, err = testpartition.WaitTxProof(t, moneyPart, tx)
	require.NoError(t, err, "transfer tx failed")
	return remainingValue
}

func createTransferTx(pubKey []byte, billID []byte, billValue uint64, fcrID []byte, timeout uint64, backlink []byte) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewBearer:   templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(pubKey)),
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
		OwnerProof: nil,
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
		OwnerProof: nil,
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
		OwnerProof: nil,
	}
	return tx, nil
}

func execBillsCommand(obsF Factory, homeDir, command string) (*testConsoleWriter, error) {
	return execCommand(obsF, homeDir, " bills "+command)
}
