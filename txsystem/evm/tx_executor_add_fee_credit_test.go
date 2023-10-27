package evm

import (
	"crypto"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill/api/predicates/templates"
	"github.com/alphabill-org/alphabill/common/hash"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/txsystem/state"

	"github.com/alphabill-org/alphabill/api/types"
	abcrypto "github.com/alphabill-org/alphabill/common/crypto"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/validator/internal/testutils/logger"
	testsig "github.com/alphabill-org/alphabill/validator/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/validator/internal/testutils/transaction"
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
	attrBytes, err := cbor.Marshal(attr)
	require.NoError(t, err)
	return &types.Payload{
		SystemID:   DefaultEvmTxSystemIdentifier,
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

func newAddFCTx(t *testing.T, unitID []byte, attr *transactions.AddFeeCreditAttributes, signer abcrypto.Signer, timeout uint64) *types.TransactionOrder {
	payload := newTxPayload(t, transactions.PayloadTypeAddFeeCredit, unitID, timeout, nil, attr)
	payloadBytes, err := payload.Bytes()
	require.NoError(t, err)
	sig, err := signer.SignBytes(payloadBytes)
	require.NoError(t, err)
	ver, err := signer.Verifier()
	require.NoError(t, err)
	pubKeyBytes, err := ver.MarshalPublicKey()
	require.NoError(t, err)
	return &types.TransactionOrder{
		Payload:    payload,
		OwnerProof: templates.NewP2pkh256SignatureBytes(sig, pubKeyBytes),
	}
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
		fc.NewDefaultFeeCreditTxValidator([]byte{0, 0, 0, 0}, DefaultEvmTxSystemIdentifier, crypto.SHA256, tb, nil))

	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name:       "err - invalid owner proof",
			args:       args{order: testfc.NewAddFC(t, signer, nil), blockNumber: 5},
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
				testfc.NewAddFCAttr(t, signer, testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(100), testfc.WithTargetRecordID(privKeyHash), testfc.WithTargetSystemID(DefaultEvmTxSystemIdentifier)),
							testtransaction.WithSystemID([]byte{0, 0, 0, 0}), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash[:]))),
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
				testfc.NewAddFCAttr(t, signer, testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(100), testfc.WithTargetRecordID(privKeyHash), testfc.WithTargetSystemID(DefaultEvmTxSystemIdentifier)),
							testtransaction.WithSystemID([]byte{0, 0, 0, 0}), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash[:]))),
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
			attr := new(transactions.AddFeeCreditAttributes)
			require.NoError(t, tt.args.order.UnmarshalAttributes(attr))
			metaData, err := addExecFn(tt.args.order, attr, tt.args.blockNumber)
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
	addFcAttr := testfc.NewAddFCAttr(t, signer, testfc.WithTransferFCTx(
		&types.TransactionRecord{
			TransactionOrder: testfc.NewTransferFC(t, nil, testtransaction.WithSystemID([]byte("not money partition"))),
			ServerMetadata:   nil,
		}))
	closeFCAmount := uint64(20)
	closeFCFee := uint64(2)
	closeFCAttr := testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(closeFCAmount))
	closureTx := testfc.WithReclaimFCClosureTx(
		&types.TransactionRecord{
			TransactionOrder: testfc.NewCloseFC(t, closeFCAttr),
			ServerMetadata:   &types.ServerMetadata{ActualFee: closeFCFee},
		},
	)
	newReclaimFCAttr := testfc.NewReclaimFCAttr(t, signer, closureTx)
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
		fc.NewDefaultFeeCreditTxValidator([]byte{0, 0, 0, 0}, DefaultEvmTxSystemIdentifier, crypto.SHA256, tb, nil))
	addFeeOrder := newAddFCTx(t,
		privKeyHash,
		testfc.NewAddFCAttr(t, signer,
			testfc.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testfc.WithTransferFCTx(
				&types.TransactionRecord{
					TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(100), testfc.WithTargetRecordID(privKeyHash), testfc.WithTargetSystemID(DefaultEvmTxSystemIdentifier)),
						testtransaction.WithSystemID([]byte{0, 0, 0, 0}), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee},
				})),
		signer, 7)
	attr := new(transactions.AddFeeCreditAttributes)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	metaData, err := addExecFn(addFeeOrder, attr, 5)
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
		testfc.NewAddFCAttr(t, signer,
			testfc.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testfc.WithTransferFCTx(
				&types.TransactionRecord{
					TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(10), testfc.WithTargetRecordID(privKeyHash), testfc.WithTargetSystemID(DefaultEvmTxSystemIdentifier), testfc.WithTargetUnitBacklink(abData.TxHash)),
						testtransaction.WithSystemID([]byte{0, 0, 0, 0}), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee},
				})),
		signer, 7)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	_, err = addExecFn(addFeeOrder, attr, 5)
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
		fc.NewDefaultFeeCreditTxValidator([]byte{0, 0, 0, 0}, DefaultEvmTxSystemIdentifier, crypto.SHA256, tb, nil))
	addFeeOrder := newAddFCTx(t,
		privKeyHash,
		testfc.NewAddFCAttr(t, signer,
			testfc.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testfc.WithTransferFCTx(
				&types.TransactionRecord{
					TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(100), testfc.WithTargetRecordID(privKeyHash), testfc.WithTargetSystemID(DefaultEvmTxSystemIdentifier)),
						testtransaction.WithSystemID([]byte{0, 0, 0, 0}), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee},
				})),
		signer, 7)
	attr := new(transactions.AddFeeCreditAttributes)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	metaData, err := addExecFn(addFeeOrder, attr, 5)
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
