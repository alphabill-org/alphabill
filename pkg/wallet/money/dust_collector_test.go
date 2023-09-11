package money

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

func TestDC_OK(t *testing.T) {
	// create wallet with 3 normal bills
	bills := []*wallet.Bill{createBill(1), createBill(2), createBill(3)}
	targetBill := bills[2]
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(billtx.DefaultSystemIdentifier, 10, backendMockWrapper.backendMock, unitLocker)

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then swap contains two dc txs
	attr := &billtx.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(&attr)
	require.NoError(t, err)
	require.EqualValues(t, 3, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestDCWontRunForSingleBill(t *testing.T) {
	// create backend with single bill
	bills := []*wallet.Bill{createBill(1)}
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(billtx.DefaultSystemIdentifier, 10, backendMockWrapper.backendMock, unitLocker)

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)

	// then swap proof is not returned
	require.Nil(t, dcResult.SwapProof)

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestAllBillsAreSwapped_WhenWalletBillCountEqualToMaxBillCount(t *testing.T) {
	// create backend with bills = max dust collection bill count
	maxBillsPerDC := 10
	bills := make([]*wallet.Bill, maxBillsPerDC)
	for i := 0; i < maxBillsPerDC; i++ {
		bills[i] = createBill(uint64(i))
	}
	targetBill := bills[maxBillsPerDC-1]
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(billtx.DefaultSystemIdentifier, maxBillsPerDC, backendMockWrapper.backendMock, unitLocker)

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)

	// then swap tx should be returned
	require.NotNil(t, dcResult.SwapProof)
	require.EqualValues(t, targetBill.GetID(), dcResult.SwapProof.TxRecord.TransactionOrder.UnitID())

	// and swap contains correct dc transfers
	swapAttr := &billtx.SwapDCAttributes{}
	swapTxo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = swapTxo.UnmarshalAttributes(swapAttr)
	require.NoError(t, err)
	require.Len(t, swapAttr.DcTransfers, maxBillsPerDC-1)
	require.Len(t, swapAttr.DcTransferProofs, maxBillsPerDC-1)
	require.EqualValues(t, 36, swapAttr.TargetValue)
	require.EqualValues(t, targetBill.GetID(), swapTxo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestOnlyFirstNBillsAreSwapped_WhenBillCountOverLimit(t *testing.T) {
	// create backend with bills = max dust collection bill count
	maxBillsPerDC := 10
	billCountInWallet := 15
	bills := make([]*wallet.Bill, billCountInWallet)
	for i := 0; i < billCountInWallet; i++ {
		bills[i] = createBill(uint64(i))
	}
	targetBill := bills[billCountInWallet-1]
	backendMockWrapper := newBackendAPIMock(t, bills)

	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(billtx.DefaultSystemIdentifier, maxBillsPerDC, backendMockWrapper.backendMock, unitLocker)

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then swap contains correct dc transfers
	swapTxo := dcResult.SwapProof.TxRecord.TransactionOrder
	swapAttr := &billtx.SwapDCAttributes{}
	err = swapTxo.UnmarshalAttributes(swapAttr)
	require.EqualValues(t, targetBill.GetID(), swapTxo.UnitID())
	require.NoError(t, err)
	require.Len(t, swapAttr.DcTransfers, maxBillsPerDC)
	require.Len(t, swapAttr.DcTransferProofs, maxBillsPerDC)
	require.EqualValues(t, 45, swapAttr.TargetValue)
	require.EqualValues(t, targetBill.GetID(), swapTxo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_OK(t *testing.T) {
	// create wallet with 2 dc bills, 1 normal bill and a locked target bill
	ctx := context.Background()
	targetBill := createBill(4)
	bills := []*wallet.Bill{
		createBill(1),
		createDCBill(2, targetBill),
		createDCBill(3, targetBill),
		targetBill,
	}
	proofs := []*wallet.Proof{
		createProofWithDCTx(t, bills[1], targetBill, 10),
		createProofWithDCTx(t, bills[2], targetBill, 10),
	}
	backendMockWrapper := newBackendAPIMock(t, bills, withProofs(proofs))
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(billtx.DefaultSystemIdentifier, 10, backendMockWrapper.backendMock, unitLocker)

	// when locked unit exists in wallet
	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		targetBill.GetTxHash(),
		unitlock.LockReasonCollectDust,
	))
	require.NoError(t, err)

	// and dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// existing dc bills should be swapped into the locked bill
	attr := &billtx.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.EqualValues(t, 5, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_UnconfirmedDCTxs_NewSwapIsSent(t *testing.T) {
	// create wallet with 3 normal bills with one of them locked
	ctx := context.Background()
	bills := []*wallet.Bill{
		createBill(1),
		createBill(2),
		createBill(3),
	}
	targetBill := bills[2]
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(billtx.DefaultSystemIdentifier, 10, backendMockWrapper.backendMock, unitLocker)

	// when locked unit exists in wallet
	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		targetBill.GetTxHash(),
		unitlock.LockReasonCollectDust,
	))
	require.NoError(t, err)

	// and dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then new swap should be sent
	attr := &billtx.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.EqualValues(t, 3, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_TargetUnitSwapIsConfirmed_ProofIsReturned(t *testing.T) {
	// create wallet with locked unit that has confirmed swap tx
	ctx := context.Background()
	bills := []*wallet.Bill{createBill(10)}
	targetBill := bills[0]
	proofs := []*wallet.Proof{createProofWithSwapTx(t, targetBill)}
	backendMockWrapper := newBackendAPIMock(t, bills, withProofs(proofs))
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(billtx.DefaultSystemIdentifier, 10, backendMockWrapper.backendMock, unitLocker)

	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		targetBill.GetTxHash(),
		unitlock.LockReasonCollectDust,
		unitlock.NewTransaction(proofs[0].TxRecord.TransactionOrder),
	))
	require.NoError(t, err)

	// when dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)

	// then confirmed swap proof is returned
	require.NotNil(t, dcResult.SwapProof)
	require.Equal(t, proofs[0], dcResult.SwapProof)

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_TargetUnitIsInvalid_NewSwapIsSent(t *testing.T) {
	// create wallet with 2 dc bills and 2 normal bills, and a target bill
	ctx := context.Background()
	targetBill := createBill(5)
	bills := []*wallet.Bill{
		createDCBill(1, targetBill),
		createDCBill(2, targetBill),
		createBill(3),
		createBill(4),
		targetBill,
	}
	proofs := []*wallet.Proof{
		createProofWithDCTx(t, bills[0], targetBill, 10),
		createProofWithDCTx(t, bills[1], targetBill, 10),
	}
	backendMockWrapper := newBackendAPIMock(t, bills, withProofs(proofs))
	unitLocker := unitlock.NewInMemoryUnitLocker()
	w := NewDustCollector(billtx.DefaultSystemIdentifier, 10, backendMockWrapper.backendMock, unitLocker)

	// lock target bill, but change tx hash so that locked unit is considered invalid
	err := unitLocker.LockUnit(unitlock.NewLockedUnit(
		backendMockWrapper.accountKey.PubKey,
		targetBill.GetID(),
		hash.Sum256(targetBill.GetTxHash()),
		unitlock.LockReasonCollectDust,
	))
	require.NoError(t, err)

	// when dc is run
	dcResult, err := w.CollectDust(ctx, backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then new swap should be sent using only the normal bill
	attr := &billtx.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.EqualValues(t, 7, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBill.GetID(), txo.UnitID())

	// and no locked units exists
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func TestExistingDC_DCOnSecondAccountDoesNotClearFirstAccountUnitLock(t *testing.T) {
	// create wallet with 3 bills
	var bills []*wallet.Bill
	for i := 0; i < 3; i++ {
		bills = append(bills, createBill(uint64(i)))
	}
	backendMockWrapper := newBackendAPIMock(t, bills)
	unitLocker := unitlock.NewInMemoryUnitLocker()

	w := NewDustCollector(billtx.DefaultSystemIdentifier, maxBillsForDustCollection, backendMockWrapper.backendMock, unitLocker)

	// lock random bill before swap
	lockedUnit := unitlock.NewLockedUnit([]byte{200}, []byte{1}, []byte{2}, unitlock.LockReasonCollectDust)
	err := unitLocker.LockUnit(lockedUnit)
	require.NoError(t, err)

	// when dc is run for account 1
	swapTx, err := w.CollectDust(context.Background(), backendMockWrapper.accountKey)
	require.NoError(t, err)
	require.NotNil(t, swapTx)

	// then the previously locked unit is not changed
	actualLockedUnit, err := unitLocker.GetUnit(lockedUnit.AccountID, lockedUnit.UnitID)
	require.NoError(t, err)
	require.Equal(t, lockedUnit, actualLockedUnit)

	// and no locked unit exists for account 1
	units, err := unitLocker.GetUnits(backendMockWrapper.accountKey.PubKey)
	require.NoError(t, err)
	require.Len(t, units, 0)
}

func createBill(value uint64) *wallet.Bill {
	return &wallet.Bill{
		Id:     util.Uint64ToBytes32(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
}

func createDCBill(value uint64, targetBill *wallet.Bill) *wallet.Bill {
	srcBill := &wallet.Bill{
		Id:     util.Uint64ToBytes32(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
	srcBill.DCTargetUnitID = targetBill.GetID()
	srcBill.DCTargetUnitBacklink = targetBill.TxHash
	return srcBill
}

type (
	dustCollectionBackendMock struct {
		backendMock *backendAPIMock
		recordedTxs map[string]*types.TransactionOrder
		accountKey  *account.AccountKey
		signer      crypto.Signer
		verifier    crypto.Verifier
		proofs      []*wallet.Proof
	}
	Options struct {
		proofs []*wallet.Proof
	}
	Option func(*Options)
)

func withProofs(proofs []*wallet.Proof) Option {
	return func(c *Options) {
		c.proofs = proofs
	}
}

func newBackendAPIMock(t *testing.T, bills []*wallet.Bill, opts ...Option) *dustCollectionBackendMock {
	recordedTxs := make(map[string]*types.TransactionOrder)
	signer, _ := testsig.CreateSignerAndVerifier(t)
	accountKeys, _ := account.NewKeys("")
	accountKey := accountKeys.AccountKey
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcb := &wallet.Bill{Id: money.NewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256), Value: 100 * 1e8}

	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return &backend.ListBillsResponse{Bills: bills}, nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			var nonDCBills []*wallet.Bill
			for _, b := range bills {
				if len(b.DCTargetUnitID) == 0 {
					nonDCBills = append(nonDCBills, b)
				}
			}
			return nonDCBills, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return fcb, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			for _, proof := range options.proofs {
				if bytes.Equal(proof.TxRecord.TransactionOrder.UnitID(), unitID) {
					return proof, nil
				}
			}
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, nil
			}
			txRecord := &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: txbuilder.MaxFee}}
			txProof := testblock.CreateProof(t, txRecord, signer)
			return &wallet.Proof{TxRecord: txRecord, TxProof: txProof}, nil
		},
	}
	return &dustCollectionBackendMock{
		backendMock: backendMock,
		recordedTxs: recordedTxs,
		signer:      signer,
		verifier:    verifier,
		accountKey:  accountKey,
	}
}

func createProofWithDCTx(t *testing.T, b *wallet.Bill, targetBill *wallet.Bill, timeout uint64) *wallet.Proof {
	keys, _ := account.NewKeys("")
	dcTx, err := txbuilder.NewDustTx(keys.AccountKey, []byte{0, 0, 0, 0}, b, targetBill, timeout)
	require.NoError(t, err)
	return createProofForTx(dcTx)
}

func createProofWithSwapTx(t *testing.T, b *wallet.Bill) *wallet.Proof {
	txo := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(b.GetID()),
		testtransaction.WithPayloadType(billtx.PayloadTypeSwapDC),
		testtransaction.WithAttributes(billtx.SwapDCAttributes{}),
	)
	return createProofForTx(txo)
}

func createProofForTx(tx *types.TransactionOrder) *wallet.Proof {
	txRecord := &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: txbuilder.MaxFee}}
	txProof := &wallet.Proof{
		TxRecord: txRecord,
		TxProof: &types.TxProof{
			BlockHeaderHash:    []byte{0},
			Chain:              []*types.GenericChainItem{{Hash: []byte{0}}},
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 10}},
		},
	}
	return txProof
}
