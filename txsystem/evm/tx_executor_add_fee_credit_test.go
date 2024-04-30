package evm

import (
	"crypto"
	"crypto/sha256"
	"math/big"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/hash"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/evm"
	fcsdk "github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"

	"github.com/stretchr/testify/require"
)

func hashOfPrivateKey(t *testing.T, signer abcrypto.Signer) []byte {
	t.Helper()
	privKeyBytes, err := signer.MarshalPrivateKey()
	require.NoError(t, err)
	h := sha256.Sum256(privKeyBytes)
	return h[:]
}

func newTxPayload(t *testing.T, txType string, unitID []byte, timeout uint64, fcrID []byte, attr interface{}) *types.Payload {
	attrBytes, err := types.Cbor.Marshal(attr)
	require.NoError(t, err)
	return &types.Payload{
		SystemID:   evm.DefaultSystemID,
		Type:       txType,
		UnitID:     unitID,
		Attributes: attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: 1,
			FeeCreditRecordID: fcrID,
		},
	}
}

func newAddFCTx(t *testing.T, unitID []byte, attr *fcsdk.AddFeeCreditAttributes, signer abcrypto.Signer, timeout uint64) *types.TransactionOrder {
	tx := &types.TransactionOrder{
		Payload: newTxPayload(t, fcsdk.PayloadTypeAddFeeCredit, unitID, timeout, nil, attr),
	}
	require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return tx
}

func evmTestFeeCalculator() uint64 {
	return 2
}

func Test_addFeeCreditTx(t *testing.T) {
	type args struct {
		order       *types.TransactionOrder
		blockNumber uint64
	}
	signer, ver := testsig.CreateSignerAndVerifier(t)
	tb := map[string]abcrypto.Verifier{"test": ver}
	pubKeyBytes, err := ver.MarshalPublicKey()
	require.NoError(t, err)
	pubHash := sha256.Sum256(pubKeyBytes)
	privKeyHash := hashOfPrivateKey(t, signer)
	addExecFn := addFeeCreditTx(
		state.NewEmptyState(),
		crypto.SHA256,
		evmTestFeeCalculator,
		fc.NewDefaultFeeCreditTxValidator(0x00000001, evm.DefaultSystemID, crypto.SHA256, tb, nil))

	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name:       "err - invalid owner proof",
			args:       args{order: testutils.NewAddFC(t, signer, nil), blockNumber: 5},
			wantErrStr: "failed to extract public key from fee credit owner proof",
		},
		{
			name:       "err - attr:transferFC tx record is nil",
			args:       args{order: newAddFCTx(t, privKeyHash, nil, signer, 7), blockNumber: 5},
			wantErrStr: "addFC tx validation failed: transferFC tx record is nil",
		},
		{
			name: "err - attr:transferFC tx record is missing server metadata",
			args: args{order: newAddFCTx(t,
				hashOfPrivateKey(t, signer),
				testutils.NewAddFCAttr(t, signer, testutils.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(100), testutils.WithTargetRecordID(privKeyHash), testutils.WithTargetSystemID(evm.DefaultSystemID)),
							testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash[:]))),
						ServerMetadata: nil,
					})),
				signer,
				7,
			),
				blockNumber: 5},
			wantErrStr: "addFC tx validation failed: transferFC tx order is missing server metadata",
		},
		{
			name: "ok",
			args: args{order: newAddFCTx(t,
				hashOfPrivateKey(t, signer),
				testutils.NewAddFCAttr(t, signer, testutils.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(100), testutils.WithTargetRecordID(privKeyHash), testutils.WithTargetSystemID(evm.DefaultSystemID)),
							testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash[:]))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					})),
				signer,
				7,
			),
				blockNumber: 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := new(fcsdk.AddFeeCreditAttributes)
			require.NoError(t, tt.args.order.UnmarshalAttributes(attr))
			metaData, err := addExecFn(tt.args.order, attr, &txsystem.TxExecutionContext{CurrentBlockNr: tt.args.blockNumber})
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				require.Nil(t, metaData)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_getTransferPayloadAttributes(t *testing.T) {
	type args struct {
		transfer *types.TransactionRecord
	}
	signer, _ := testsig.CreateSignerAndVerifier(t)
	addFcAttr := testutils.NewAddFCAttr(t, signer, testutils.WithTransferFCTx(
		&types.TransactionRecord{
			TransactionOrder: testutils.NewTransferFC(t, nil, testtransaction.WithSystemID(0xFFFFFFFF)),
			ServerMetadata:   nil,
		}))
	closeFCAmount := uint64(20)
	closeFCFee := uint64(2)
	closeFCAttr := testutils.NewCloseFCAttr(testutils.WithCloseFCAmount(closeFCAmount))
	closureTx := testutils.WithReclaimFCClosureTx(
		&types.TransactionRecord{
			TransactionOrder: testutils.NewCloseFC(t, closeFCAttr),
			ServerMetadata:   &types.ServerMetadata{ActualFee: closeFCFee},
		},
	)
	newReclaimFCAttr := testutils.NewReclaimFCAttr(t, signer, closureTx)
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "nil",
			args:    args{transfer: nil},
			wantErr: true,
		},
		{
			name:    "incorrect type",
			args:    args{transfer: newReclaimFCAttr.CloseFeeCreditTransfer},
			wantErr: true,
		},
		{
			name:    "ok",
			args:    args{transfer: addFcAttr.FeeCreditTransfer},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getTransferPayloadAttributes(tt.args.transfer)
			if (err != nil) != tt.wantErr {
				t.Errorf("getTransferPayloadAttributes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_addFeeCreditTxAndUpdate(t *testing.T) {
	const transferFcFee = 1
	stateTree := state.NewEmptyState()
	signer, ver := testsig.CreateSignerAndVerifier(t)
	tb := map[string]abcrypto.Verifier{"test": ver}
	pubKeyBytes, err := ver.MarshalPublicKey()
	require.NoError(t, err)
	pubHash := hash.Sum256(pubKeyBytes)
	privKeyHash := hashOfPrivateKey(t, signer)
	addExecFn := addFeeCreditTx(
		stateTree,
		crypto.SHA256,
		evmTestFeeCalculator,
		fc.NewDefaultFeeCreditTxValidator(0x00000001, evm.DefaultSystemID, crypto.SHA256, tb, nil))
	addFeeOrder := newAddFCTx(t,
		privKeyHash,
		testutils.NewAddFCAttr(t, signer,
			testutils.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testutils.WithTransferFCTx(
				&types.TransactionRecord{
					TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(100), testutils.WithTargetRecordID(privKeyHash), testutils.WithTargetSystemID(evm.DefaultSystemID)),
						testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee},
				})),
		signer, 7)
	attr := new(fcsdk.AddFeeCreditAttributes)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	metaData, err := addExecFn(addFeeOrder, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 5})
	require.NoError(t, err)
	require.NotNil(t, metaData)
	require.EqualValues(t, evmTestFeeCalculator(), metaData.ActualFee)
	// validate stateDB
	stateDB := statedb.NewStateDB(stateTree, logger.New(t))
	addr, err := generateAddress(pubKeyBytes)
	require.NoError(t, err)
	balance := stateDB.GetBalance(addr)
	// balance is equal to 100 - "transfer fee" - "add fee" to wei
	remainingCredit := new(big.Int).Sub(alphaToWei(100), alphaToWei(transferFcFee))
	remainingCredit = new(big.Int).Sub(remainingCredit, alphaToWei(evmTestFeeCalculator()))
	require.EqualValues(t, balance, remainingCredit)
	// check owner condition set
	u, err := stateTree.GetUnit(addr.Bytes(), false)
	require.NoError(t, err)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(pubHash), u.Bearer())

	abData := stateDB.GetAlphaBillData(addr)
	// add more funds
	addFeeOrder = newAddFCTx(t,
		privKeyHash,
		testutils.NewAddFCAttr(t, signer,
			testutils.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testutils.WithTransferFCTx(
				&types.TransactionRecord{
					TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(10), testutils.WithTargetRecordID(privKeyHash), testutils.WithTargetSystemID(evm.DefaultSystemID), testutils.WithTargetUnitBacklink(abData.TxHash)),
						testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee},
				})),
		signer, 7)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	_, err = addExecFn(addFeeOrder, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 5})
	require.NoError(t, err)
	remainingCredit = new(big.Int).Add(remainingCredit, alphaToWei(10))
	balance = stateDB.GetBalance(addr)
	// balance is equal to remaining+10-"transfer fee 1" -"ass fee = 2" to wei
	remainingCredit = new(big.Int).Sub(remainingCredit, alphaToWei(transferFcFee))
	remainingCredit = new(big.Int).Sub(remainingCredit, alphaToWei(evmTestFeeCalculator()))
	require.EqualValues(t, balance, remainingCredit)
	// check owner condition
	u, err = stateTree.GetUnit(addr.Bytes(), false)
	require.NoError(t, err)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(pubHash), u.Bearer())
}

func Test_addFeeCreditTxToExistingAccount(t *testing.T) {
	const transferFcFee = 1
	stateTree := state.NewEmptyState()
	signer, ver := testsig.CreateSignerAndVerifier(t)
	tb := map[string]abcrypto.Verifier{"test": ver}
	pubKeyBytes, err := ver.MarshalPublicKey()
	require.NoError(t, err)
	address, err := generateAddress(pubKeyBytes)
	require.NoError(t, err)
	stateDB := statedb.NewStateDB(stateTree, logger.New(t))
	stateDB.CreateAccount(address)
	stateDB.AddBalance(address, alphaToWei(100))
	pubHash := hash.Sum256(pubKeyBytes)
	privKeyHash := hashOfPrivateKey(t, signer)
	addExecFn := addFeeCreditTx(
		stateTree,
		crypto.SHA256,
		evmTestFeeCalculator,
		fc.NewDefaultFeeCreditTxValidator(0x00000001, evm.DefaultSystemID, crypto.SHA256, tb, nil))
	addFeeOrder := newAddFCTx(t,
		privKeyHash,
		testutils.NewAddFCAttr(t, signer,
			testutils.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testutils.WithTransferFCTx(
				&types.TransactionRecord{
					TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(100), testutils.WithTargetRecordID(privKeyHash), testutils.WithTargetSystemID(evm.DefaultSystemID)),
						testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee},
				})),
		signer, 7)
	attr := new(fcsdk.AddFeeCreditAttributes)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	metaData, err := addExecFn(addFeeOrder, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 5})
	require.NoError(t, err)
	require.NotNil(t, metaData)
	require.EqualValues(t, evmTestFeeCalculator(), metaData.ActualFee)
	// validate stateDB
	balance := stateDB.GetBalance(address)
	// balance is equal to 100+100 - "transfer fee" - "add fee" to wei
	remainingCredit := new(big.Int).Sub(alphaToWei(200), alphaToWei(transferFcFee))
	remainingCredit = new(big.Int).Sub(remainingCredit, alphaToWei(evmTestFeeCalculator()))
	require.EqualValues(t, balance, remainingCredit)
	// check owner condition as well
	u, err := stateTree.GetUnit(address.Bytes(), false)
	require.NoError(t, err)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(pubHash), u.Bearer())
}
