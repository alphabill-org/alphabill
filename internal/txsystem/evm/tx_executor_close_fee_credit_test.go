package evm

import (
	"crypto"
	"crypto/sha256"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

func newCloseFCTx(t *testing.T, unitID []byte, attr *transactions.CloseFeeCreditAttributes, signer abcrypto.Signer, timeout uint64) *types.TransactionOrder {
	payload := newTxPayload(t, transactions.PayloadTypeCloseFeeCredit, unitID, timeout, nil, attr)
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
		OwnerProof: script.PredicateArgumentPayToPublicKeyHashDefault(sig, pubKeyBytes),
	}
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
		fc.NewDefaultFeeCreditTxValidator([]byte{0, 0, 0, 0}, DefaultEvmTxSystemIdentifier, crypto.SHA256, tb))
	addFeeOrder := newAddFCTx(t,
		privKeyHash,
		testfc.NewAddFCAttr(t, signer, testfc.WithTransferFCTx(
			&types.TransactionRecord{
				TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(amount), testfc.WithTargetRecordID(privKeyHash), testfc.WithTargetSystemID(DefaultEvmTxSystemIdentifier)),
					testtransaction.WithSystemID([]byte{0, 0, 0, 0}), testtransaction.WithOwnerProof(script.PredicatePayToPublicKeyHashDefault(pubHash[:]))),
				ServerMetadata: &types.ServerMetadata{ActualFee: 1},
			})),
		signer, 7)
	backlink := addFeeOrder.Hash(crypto.SHA256)
	attr := new(transactions.AddFeeCreditAttributes)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	metaData, err := addExecFn(addFeeOrder, attr, 5)
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
		evmTestFeeCalculator,
		fc.NewDefaultFeeCreditTxValidator([]byte{0, 0, 0, 0}, DefaultEvmTxSystemIdentifier, crypto.SHA256, tb))

	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name:       "err - invalid owner proof",
			args:       args{order: testfc.NewCloseFC(t, nil), blockNumber: 5},
			wantErrStr: "failed to extract public key from fee credit owner proof",
		},
		{
			name:       "err - attr:nil - amount is 0 and not 98",
			args:       args{order: newCloseFCTx(t, privKeyHash, nil, signer, 7), blockNumber: 5},
			wantErrStr: "closeFC: tx validation failed: invalid amount: amount=0 fcr.Balance=98",
		},
		{
			name: "err - no unit (no credit has been added)",
			args: args{order: newCloseFCTx(t,
				test.RandomBytes(32),
				testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(uint64(98)),
					testfc.WithCloseFCTargetUnitID(privKeyHash), testfc.WithCloseFCTargetUnitBacklink(backlink)),
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
			metaData, err := closeExecFn(tt.args.order, attr, tt.args.blockNumber)
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
	stateDB := statedb.NewStateDB(stateTree)
	addr, err := generateAddress(pubKeyBytes)
	require.NoError(t, err)
	balance := stateDB.GetBalance(addr)
	balanceAlpha := weiToAlpha(balance)
	// close fee credit
	closeExecFn := closeFeeCreditTx(
		stateTree,
		evmTestFeeCalculator,
		fc.NewDefaultFeeCreditTxValidator([]byte{0, 0, 0, 0}, DefaultEvmTxSystemIdentifier, crypto.SHA256, tb))
	// create close order
	closeOrder := newCloseFCTx(t, test.RandomBytes(32), testfc.NewCloseFCAttr(
		testfc.WithCloseFCAmount(balanceAlpha),
		testfc.WithCloseFCTargetUnitBacklink(backlink),
		testfc.WithCloseFCTargetUnitID(privKeyHash)),
		signer, 7)
	closeAttr := new(transactions.CloseFeeCreditAttributes)
	require.NoError(t, closeOrder.UnmarshalAttributes(closeAttr))
	// first add fee credit
	metaData, err := closeExecFn(closeOrder, closeAttr, 5)
	require.NoError(t, err)
	require.NotNil(t, metaData)
	// verify balance
	balance = stateDB.GetBalance(addr)
	require.EqualValues(t, 0, balance.Uint64())
}
