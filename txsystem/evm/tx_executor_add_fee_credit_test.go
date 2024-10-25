package evm

import (
	"crypto"
	"crypto/sha256"
	"fmt"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestAddFC_ValidateAddNewFeeCreditTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	fcrID := testfc.NewFeeCreditRecordID(t, signer)

	txRecord := &types.TransactionRecord{Version: 1,
		TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetSystemID(evm.DefaultSystemID)))),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
	}
	txRecordProof := testblock.CreateTxRecordProof(t, txRecord, signer)

	t.Run("ok - empty", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer, testfc.WithTransferFCProof(txRecordProof)),
		)
		ownerProof := testsig.NewAuthProofSignature(t, tx, signer)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: ownerProof}
		require.NoError(t, tx.SetAuthProof(authProof))

		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))))
	})
	t.Run("err - replay will not pass validation", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(txRecordProof),
			),
		)
		ownerProof := testsig.NewAuthProofSignature(t, tx, signer)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: ownerProof}
		require.NoError(t, tx.SetAuthProof(authProof))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10))))
		sm, err := feeCreditModule.executeAddFC(tx, &attr, &authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
		require.NoError(t, err)
		require.NotNil(t, sm)
		// replay attack
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10))),
			"invalid transferFC target unit counter: transferFC.targetUnitCounter=<nil> unit.counter=0")
	})
	t.Run("transferFC transaction record is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithAttributes(&fcsdk.AddFeeCreditAttributes{
				FeeCreditTransferProof: &types.TxRecordProof{},
			}),
		)
		ownerProof := testsig.NewAuthProofSignature(t, tx, signer)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: ownerProof}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"transferFC transaction record proof is not valid: transaction record is nil")
	})
	t.Run("transferFC transaction order is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithAttributes(&fcsdk.AddFeeCreditAttributes{FeeCreditTransferProof: &types.TxRecordProof{TxRecord: &types.TransactionRecord{Version: 1}}}),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"transferFC transaction record proof is not valid: transaction order is nil")
	})
	t.Run("transferFC proof is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fcsdk.AddFeeCreditAttributes{
				FeeCreditTransferProof: &types.TxRecordProof{
					TxRecord: &types.TransactionRecord{Version: 1, TransactionOrder: testtransaction.TxoToBytes(t, &types.TransactionOrder{}), ServerMetadata: &types.ServerMetadata{}},
				},
			},
		))
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"transferFC transaction record proof is not valid: transaction proof is nil")
	})
	t.Run("transferFC server metadata is nil", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(&types.TxRecordProof{
					TxRecord: &types.TransactionRecord{Version: 1,
						TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetUnitCounter(4)))),
						ServerMetadata:   nil,
					},
				}),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"transferFC transaction record proof is not valid: server metadata is nil")
	})
	t.Run("transferFC attributes unmarshal error", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewAddFC(t, signer, nil,
				testtransaction.WithTransactionType(fcsdk.TransactionTypeTransferFeeCredit))),
			ServerMetadata: &types.ServerMetadata{},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"failed to unmarshal transfer fee credit attributes: cbor: cannot unmarshal array into Go value of type fc.TransferFeeCreditAttributes (cannot decode CBOR array to struct with different number of elements)")
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{1, 2, 3}}),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"invalid fee credit transaction: fee transaction cannot contain fee credit reference")
	})
	t.Run("bill not transferred to fee credits of the target record", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetSystemID(evm.DefaultSystemID)))),
			ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
			testtransaction.WithUnitID([]byte{1}),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			fmt.Sprintf("invalid transferFC target record id: transferFC.TargetRecordId=%s tx.UnitId=01", fcrID))
	})
	t.Run("Invalid fee credit owner predicate", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetSystemID(evm.DefaultSystemID)))),
			ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
				testfc.WithFeeCreditOwnerPredicate([]byte("wrong bearer")),
			))
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: []byte("wrong owner proof")}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID, templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{}))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"failed to extract public key from fee credit owner proof")
	})
	t.Run("invalid system id", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer, nil, testtransaction.WithSystemID(0xFFFFFFFF))),
			ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"invalid transferFC money system identifier 4294967295 (expected 1)")
	})
	t.Run("Invalid target systemID", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetSystemID(0xFFFFFFFF)))),
			ServerMetadata:   &types.ServerMetadata{},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"invalid transferFC target system identifier: expected_target_system_id: 00000003 actual_target_system_id=FFFFFFFF")
	})
	t.Run("Invalid target recordID", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
				testfc.NewTransferFCAttr(t, signer,
					testfc.WithTargetSystemID(evm.DefaultSystemID),
					testfc.WithTargetRecordID([]byte("not equal to transaction.unitId"))))),
			ServerMetadata: &types.ServerMetadata{},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			fmt.Sprintf("invalid transferFC target record id: transferFC.TargetRecordId=6E6F7420657175616C20746F207472616E73616374696F6E2E756E69744964 tx.UnitId=%s", fcrID))
	})
	t.Run("invalid target unit counter (fee credit record does not exist)", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
				testfc.NewTransferFCAttr(t, signer,
					testfc.WithTargetSystemID(evm.DefaultSystemID),
					testfc.WithTargetUnitCounter(4)))),
			ServerMetadata: &types.ServerMetadata{ActualFee: 1},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"invalid transferFC target unit counter: transferFC.targetUnitCounter=4 unit.counter=<nil>")
	})
	t.Run("invalid target unit counter (tx.targetUnitCounter < unit.counter)", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
				testfc.NewTransferFCAttr(t, signer,
					testfc.WithTargetSystemID(evm.DefaultSystemID),
					testfc.WithTargetUnitCounter(4)))),
			ServerMetadata: &types.ServerMetadata{ActualFee: 1},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"invalid transferFC target unit counter: transferFC.targetUnitCounter=4 unit.counter=<nil>")
	})
	t.Run("ok target unit counter (tx target unit counter equals fee credit record counter)", func(t *testing.T) {
		t.SkipNow() // TODO fix EVM fee credit records
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
				testfc.NewTransferFCAttr(t, signer,
					testfc.WithTargetSystemID(evm.DefaultSystemID),
					testfc.WithTargetUnitCounter(4)),
				testtransaction.WithUnitID(fcrID))),
			ServerMetadata: &types.ServerMetadata{ActualFee: 1},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		address, err := generateAddress(pubKey)
		require.NoError(t, err)
		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: uint256.NewInt(100)},
				AlphaBill: &statedb.AlphaBillLink{Counter: 4},
			}))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx))
	})
	t.Run("LatestAdditionTime in the past NOK", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
				testfc.NewTransferFCAttr(t, signer,
					testfc.WithTargetSystemID(evm.DefaultSystemID),
					testfc.WithLatestAdditionTime(9)),
				testtransaction.WithUnitID(fcrID))),
			ServerMetadata: &types.ServerMetadata{},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(10))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"invalid transferFC timeout: latestAdditionTime=9 currentRoundNumber=10")
	})
	t.Run("LatestAdditionTime next block OK", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
				testfc.NewTransferFCAttr(t, signer,
					testfc.WithTargetSystemID(evm.DefaultSystemID),
					testfc.WithAmount(100)))),
			ServerMetadata: &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 90}),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(10))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx))
	})
	t.Run("Invalid transaction fee", func(t *testing.T) {
		txr := &types.TransactionRecord{Version: 1,
			TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
				testfc.NewTransferFCAttr(t, signer,
					testfc.WithTargetSystemID(evm.DefaultSystemID),
					testfc.WithAmount(100)))),
			ServerMetadata: &types.ServerMetadata{ActualFee: 1},
		}
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, txr, signer)),
			),
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 101}),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"invalid transferFC fee: max_fee+actual_fee=102 transferFC.Amount=100")
	})
	t.Run("Invalid proof", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(newInvalidProof(t, signer)),
			),
		)
		authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, &authProof, execCtx),
			"proof is not valid: proof block hash does not match to block hash in unicity certificate")
	})
}

func Test_addFeeCreditTxAndUpdate(t *testing.T) {
	const transferFcFee = 1
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	pubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	pubHash := hash.Sum256(pubKeyBytes)
	privKeyHash := hashOfPrivateKey(t, signer)
	feeCreditModule := newTestFeeModule(t, trustBase)
	transFcRecord := &types.TransactionRecord{Version: 1,
		TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
			testfc.NewTransferFCAttr(t, signer,
				testfc.WithAmount(100),
				testfc.WithTargetRecordID(privKeyHash),
				testfc.WithTargetSystemID(evm.DefaultSystemID),
			),
			testtransaction.WithSystemID(0x00000001),
			testtransaction.WithAuthProof(fcsdk.TransferFeeCreditAuthProof{OwnerProof: templates.NewP2pkh256BytesFromKeyHash(pubHash)}))),
		ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee, SuccessIndicator: types.TxStatusSuccessful},
	}
	addFeeOrder := newAddFCTx(t,
		privKeyHash,
		testfc.NewAddFCAttr(t, signer,
			testfc.WithFeeCreditOwnerPredicate(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, transFcRecord, signer))),
		signer, 7)
	attr := new(fcsdk.AddFeeCreditAttributes)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	authProof := new(fcsdk.AddFeeCreditAuthProof)
	require.NoError(t, addFeeOrder.UnmarshalAuthProof(authProof))

	require.NoError(t, feeCreditModule.validateAddFC(addFeeOrder, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))))
	metaData, err := feeCreditModule.executeAddFC(addFeeOrder, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(5)))
	require.NoError(t, err)
	require.NotNil(t, metaData)
	require.EqualValues(t, evmTestFeeCalculator(), metaData.ActualFee)
	// validate stateDB
	stateDB := statedb.NewStateDB(feeCreditModule.state, logger.New(t))
	addr, err := generateAddress(pubKeyBytes)
	require.NoError(t, err)
	balance := stateDB.GetBalance(addr)
	// balance is equal to 100 - "transfer fee" - "add fee" to wei
	remainingCredit := new(uint256.Int).Sub(alphaToWei(100), alphaToWei(transferFcFee))
	remainingCredit = new(uint256.Int).Sub(remainingCredit, alphaToWei(evmTestFeeCalculator()))
	require.EqualValues(t, balance, remainingCredit)
	// check owner predicate set
	u, err := feeCreditModule.state.GetUnit(addr.Bytes(), false)
	require.NoError(t, err)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(pubHash), u.Data().Owner())

	abData := stateDB.GetAlphaBillData(addr)
	// add more funds
	transFcRecord = &types.TransactionRecord{Version: 1,
		TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
			testfc.NewTransferFCAttr(t, signer,
				testfc.WithAmount(10),
				testfc.WithTargetRecordID(privKeyHash),
				testfc.WithTargetSystemID(evm.DefaultSystemID),
				testfc.WithTargetUnitCounter(abData.Counter),
			),
			testtransaction.WithSystemID(0x00000001),
			testtransaction.WithAuthProof(fcsdk.TransferFeeCreditAuthProof{OwnerProof: templates.NewP2pkh256BytesFromKeyHash(pubHash)}))),
		ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee, SuccessIndicator: types.TxStatusSuccessful},
	}
	addFeeOrder = newAddFCTx(t,
		privKeyHash,
		testfc.NewAddFCAttr(t, signer,
			testfc.WithFeeCreditOwnerPredicate(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testfc.WithTransferFCProof(testblock.CreateTxRecordProof(t, transFcRecord, signer))),
		signer, 7)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	require.NoError(t, feeCreditModule.validateAddFC(addFeeOrder, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))))
	metaData, err = feeCreditModule.executeAddFC(addFeeOrder, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(5)))
	require.NoError(t, err)
	require.NotNil(t, metaData)
	remainingCredit = new(uint256.Int).Add(remainingCredit, alphaToWei(10))
	balance = stateDB.GetBalance(addr)
	// balance is equal to remaining+10-"transfer fee 1" -"ass fee = 2" to wei
	remainingCredit = new(uint256.Int).Sub(remainingCredit, alphaToWei(transferFcFee))
	remainingCredit = new(uint256.Int).Sub(remainingCredit, alphaToWei(evmTestFeeCalculator()))
	require.EqualValues(t, balance, remainingCredit)
	// check owner predicate
	u, err = feeCreditModule.state.GetUnit(addr.Bytes(), false)
	require.NoError(t, err)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(pubHash), u.Data().Owner())
}

type feeTestOption func(m *FeeAccount) error

func withStateUnit(unitID []byte, bearer types.PredicateBytes, data types.UnitData) feeTestOption {
	return func(m *FeeAccount) error {
		return m.state.Apply(state.AddUnit(unitID, data))
	}
}

func newTestFeeModule(t *testing.T, tb types.RootTrustBase, opts ...feeTestOption) *FeeAccount {
	m := &FeeAccount{
		hashAlgorithm: crypto.SHA256,
		feeCalculator: FixedFee(1),
		state:         state.NewEmptyState(),
		systemID:      evm.DefaultSystemID,
		moneySystemID: money.DefaultSystemID,
		trustBase:     tb,
	}
	for _, o := range opts {
		require.NoError(t, o(m))
	}
	return m
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxRecordProof {
	txRecord := &types.TransactionRecord{Version: 1,
		TransactionOrder: testtransaction.TxoToBytes(t, testfc.NewTransferFC(t, signer,
			testfc.NewTransferFCAttr(t, signer,
				testfc.WithTargetSystemID(evm.DefaultSystemID),
				testfc.WithAmount(100)),
		)),
		ServerMetadata: &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
	}
	txRecordProof := testblock.CreateTxRecordProof(t, txRecord, signer)
	txRecordProof.TxProof.BlockHeaderHash = []byte("invalid hash")
	return txRecordProof
}

func hashOfPrivateKey(t *testing.T, signer abcrypto.Signer) []byte {
	t.Helper()
	privKeyBytes, err := signer.MarshalPrivateKey()
	require.NoError(t, err)
	h := sha256.Sum256(privKeyBytes)
	return h[:]
}

func newTxPayload(t *testing.T, txType uint16, unitID []byte, timeout uint64, fcrID []byte, attr interface{}) types.Payload {
	attrBytes, err := types.Cbor.Marshal(attr)
	require.NoError(t, err)
	return types.Payload{
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
		Payload: newTxPayload(t, fcsdk.TransactionTypeAddFeeCredit, unitID, timeout, nil, attr),
	}
	ownerProof := testsig.NewAuthProofSignature(t, tx, signer)
	require.NoError(t, tx.SetAuthProof(fcsdk.AddFeeCreditAuthProof{OwnerProof: ownerProof}))
	return tx
}

func evmTestFeeCalculator() uint64 {
	return 1
}
