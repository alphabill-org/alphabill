package evm

import (
	"crypto"
	"crypto/sha256"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	fct "github.com/alphabill-org/alphabill/txsystem/fc/types"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func newCloseFCTx(t *testing.T, unitID []byte, attr *transactions.CloseFeeCreditAttributes, signer abcrypto.Signer, timeout uint64) *types.TransactionOrder {
	tx := &types.TransactionOrder{
		Payload: newTxPayload(t, transactions.PayloadTypeCloseFeeCredit, unitID, timeout, nil, attr),
	}
	require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return tx
}

func addFeeCredit(t *testing.T, tree *state.State, signer abcrypto.Signer, amount uint64) []byte {
	t.Helper()
	ver, err := signer.Verifier()
	require.NoError(t, err)
	pubKeyBytes, err := ver.MarshalPublicKey()
	require.NoError(t, err)
	pubHash := sha256.Sum256(pubKeyBytes)
	privKeyHash := hashOfPrivateKey(t, signer)
	tb := map[string]abcrypto.Verifier{"test": ver}

	addExecFn := addFeeCreditTx(
		tree,
		crypto.SHA256,
		evmTestFeeCalculator,
		fc.NewDefaultFeeCreditTxValidator(0x00000001, DefaultEvmTxSystemIdentifier, crypto.SHA256, tb, nil))
	addFeeOrder := newAddFCTx(t,
		privKeyHash,
		testutils.NewAddFCAttr(t, signer, testutils.WithTransferFCTx(
			&types.TransactionRecord{
				TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(fct.Fee(amount)), testutils.WithTargetRecordID(privKeyHash), testutils.WithTargetSystemID(DefaultEvmTxSystemIdentifier)),
					testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash[:]))),
				ServerMetadata: &types.ServerMetadata{ActualFee: 1},
			})),
		signer, 7)
	backlink := addFeeOrder.Hash(crypto.SHA256)
	attr := new(transactions.AddFeeCreditAttributes)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	metaData, err := addExecFn(addFeeOrder, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 5})
	require.NotNil(t, metaData)
	require.EqualValues(t, evmTestFeeCalculator(), metaData.ActualFee)
	require.NoError(t, err)
	return backlink
}

func Test_closeFeeCreditTxExecFn(t *testing.T) {
	type args struct {
		order       *types.TransactionOrder
		blockNumber uint64
	}
	stateTree := state.NewEmptyState()
	signer, ver := testsig.CreateSignerAndVerifier(t)
	tb := map[string]abcrypto.Verifier{"test": ver}
	privKeyHash := hashOfPrivateKey(t, signer)
	backlink := addFeeCredit(t, stateTree, signer, 100)
	closeExecFn := closeFeeCreditTx(
		stateTree,
		crypto.SHA256,
		evmTestFeeCalculator,
		fc.NewDefaultFeeCreditTxValidator(0x00000001, DefaultEvmTxSystemIdentifier, crypto.SHA256, tb, nil),
		logger.New(t))

	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name:       "err - invalid owner proof",
			args:       args{order: testutils.NewCloseFC(t, nil), blockNumber: 5},
			wantErrStr: "failed to extract public key from fee credit owner proof",
		},
		{
			name:       "err - attr:nil - amount is 0 and not 98",
			args:       args{order: newCloseFCTx(t, privKeyHash, nil, signer, 7), blockNumber: 5},
			wantErrStr: "closeFC: tx validation failed: invalid amount: amount=0 fcr.Balance=97",
		},
		{
			name: "err - no unit (no credit has been added)",
			args: args{order: newCloseFCTx(t,
				test.RandomBytes(32),
				testutils.NewCloseFCAttr(testutils.WithCloseFCAmount(97),
					testutils.WithCloseFCTargetUnitID(privKeyHash), testutils.WithCloseFCTargetUnitBacklink(backlink)),
				signer,
				7,
			),
				blockNumber: 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := new(transactions.CloseFeeCreditAttributes)
			require.NoError(t, tt.args.order.UnmarshalAttributes(attr))
			metaData, err := closeExecFn(tt.args.order, attr, &txsystem.TxExecutionContext{CurrentBlockNr: tt.args.blockNumber})
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				require.Nil(t, metaData)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_closeFeeCreditTx(t *testing.T) {
	stateTree := state.NewEmptyState()
	signer, ver := testsig.CreateSignerAndVerifier(t)
	tb := map[string]abcrypto.Verifier{"test": ver}
	pubKeyBytes, err := ver.MarshalPublicKey()
	require.NoError(t, err)
	privKeyHash := hashOfPrivateKey(t, signer)
	backlink := addFeeCredit(t, stateTree, signer, 100)
	log := logger.New(t)
	stateDB := statedb.NewStateDB(stateTree, log)
	addr, err := generateAddress(pubKeyBytes)
	require.NoError(t, err)
	balance := stateDB.GetBalance(addr)
	balanceAlpha := weiToAlpha(balance)
	// close fee credit
	closeExecFn := closeFeeCreditTx(
		stateTree,
		crypto.SHA256,
		evmTestFeeCalculator,
		fc.NewDefaultFeeCreditTxValidator(0x00000001, DefaultEvmTxSystemIdentifier, crypto.SHA256, tb, nil),
		log)
	// create close order
	closeOrder := newCloseFCTx(t, test.RandomBytes(32), testutils.NewCloseFCAttr(
		testutils.WithCloseFCAmount(balanceAlpha),
		testutils.WithCloseFCTargetUnitBacklink(backlink),
		testutils.WithCloseFCTargetUnitID(privKeyHash)),
		signer, 7)
	closeAttr := new(transactions.CloseFeeCreditAttributes)
	require.NoError(t, closeOrder.UnmarshalAttributes(closeAttr))
	// first add fee credit
	metaData, err := closeExecFn(closeOrder, closeAttr, &txsystem.TxExecutionContext{CurrentBlockNr: 5})
	require.NoError(t, err)
	require.NotNil(t, metaData)
	// verify balance
	balance = stateDB.GetBalance(addr)
	require.EqualValues(t, 0, balance.Uint64())
	// verify backlink
	alphaBillData := stateDB.GetAlphaBillData(addr)
	require.Equal(t, closeOrder.Hash(crypto.SHA256), alphaBillData.TxHash)
}
