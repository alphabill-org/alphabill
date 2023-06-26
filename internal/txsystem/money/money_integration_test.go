package money

import (
	"bytes"
	"crypto"
	"sort"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testmoneyfc "github.com/alphabill-org/alphabill/internal/testutils/money"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

var (
	systemIdentifier = []byte{0, 0, 0, 0}

	pubKey1  = "0x0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"
	privKey1 = "0xa5e8bff9733ebc751a45ca4b8cc6ce8e76c8316a5eb556f738092df6232e78de"

	pubKey2  = "0x02d29cbdea6062c0a9d9170245188fa39a12ad3dd6cc02a78fcc026594d9bdc06c"
	privKey2 = "0xd7e5041766e8ca505ab07ffa46652e248ede22b436ec81b583a78c8c9e1aac6b"
)

func TestPartition_Ok(t *testing.T) {
	const moneyInvariant = uint64(10000 * 1e8)
	total := moneyInvariant
	initialBill := &InitialBill{
		ID:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		Value: moneyInvariant,
		Owner: script.PredicateAlwaysTrue(),
	}
	txFee := fc.FixedFee(1)
	moneyPrt, err := testpartition.NewPartition(3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := NewMoneyTxSystem(systemIdentifier,
			WithHashAlgorithm(crypto.SHA256),
			WithInitialBill(initialBill),
			WithSystemDescriptionRecords(createSDRs(2)),
			WithDCMoneyAmount(0),
			WithTrustBase(tb),
			WithFeeCalculator(fc.FixedFee(1)),
		)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{moneyPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })

	// create fee credit for initial bill transfer
	fcrAmount := testmoneyfc.FCRAmount
	transferFC := testmoneyfc.CreateFeeCredit(t, initialBill.ID, abNet)

	// transfer initial bill to pubKey1
	transferInitialBillTx, _ := createBillTransfer(t, initialBill.ID, total-fcrAmount-txFee(), script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey1)), transferFC.Hash(crypto.SHA256))
	err = moneyPrt.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPrt, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	_, _, transferInitialBillTxRecord, err := moneyPrt.GetTxProof(transferInitialBillTx)
	require.NoError(t, err)

	// split initial bill from pubKey1 to pubKey2
	amountPK2 := uint64(1000)
	tx := createSplitTx(t, initialBill.ID, transferInitialBillTxRecord, amountPK2, total-fcrAmount-txFee()-amountPK2)
	err = moneyPrt.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPrt, tx), test.WaitDuration, test.WaitTick)

	// wrong partition tx
	tx = createSplitTx(t, initialBill.ID, transferInitialBillTxRecord, amountPK2, total-fcrAmount-txFee()-amountPK2)
	tx.Payload.SystemID = []byte{1, 1, 1, 1}
	err = moneyPrt.SubmitTx(tx)
	require.Error(t, err)
	require.Never(t, testpartition.BlockchainContainsTx(moneyPrt, tx), test.WaitDuration, test.WaitTick)
}

func TestPartition_SwapDCOk(t *testing.T) {
	const moneyInvariant = uint64(10000 * 1e8)
	const nofDustToSwap = 3

	var (
		hashAlgorithm = crypto.SHA256
		txsState      *state.State
		initialBill   = &InitialBill{
			ID:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			Value: moneyInvariant,
			Owner: script.PredicateAlwaysTrue(),
		}
		feeFunc = fc.FixedFee(1)
	)
	total := moneyInvariant
	moneyPrt, err := testpartition.NewPartition(3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		var err error
		txsState = state.NewEmptyState()
		//	trustBase = tb
		system, err := NewMoneyTxSystem(systemIdentifier,
			WithHashAlgorithm(crypto.SHA256),
			WithInitialBill(initialBill),
			WithSystemDescriptionRecords(createSDRs(2)),
			WithDCMoneyAmount(100),
			WithTrustBase(tb),
			WithState(txsState),
			WithFeeCalculator(fc.FixedFee(1)),
		)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{moneyPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })

	// create fee credit for initial bill transfer
	txFee := feeFunc()
	fcrAmount := testmoneyfc.FCRAmount
	transferFC := testmoneyfc.CreateFeeCredit(t, initialBill.ID, abNet)
	require.NoError(t, err)

	// transfer initial bill to pubKey1
	transferInitialBillTx, _ := createBillTransfer(t, initialBill.ID, total-fcrAmount-txFee, script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey1)), transferFC.Hash(hashAlgorithm))
	require.NoError(t, moneyPrt.SubmitTx(transferInitialBillTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPrt, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// split initial bill, create small payments from which to make dust payments
	splitTxs := make([]*types.TransactionRecord, nofDustToSwap)
	amount := uint64(1)
	_, _, transferRecord, err := moneyPrt.GetTxProof(transferInitialBillTx)
	require.NoError(t, err)
	var prev = transferRecord
	total = total - fcrAmount - txFee
	for i := range splitTxs {
		total = total - amount
		splitTx := createSplitTx(t, initialBill.ID, prev, amount, total)
		require.NoError(t, moneyPrt.SubmitTx(splitTx))
		// wait for transaction to be added to block
		require.Eventually(t, testpartition.BlockchainContainsTx(moneyPrt, splitTx), test.WaitDuration, test.WaitTick)
		_, _, record, err := moneyPrt.GetTxProof(splitTx)
		require.NoError(t, err)
		prev = record
		splitTxs[i] = record
		amount++
	}

	// create dust payments from splits
	dcBillIds := make([]types.UnitID, len(splitTxs))
	for i, splitTx := range splitTxs {
		dcBillIds[i] = txutil.SameShardID(splitTx.TransactionOrder.UnitID(), unitIdFromTransaction(splitTx.TransactionOrder))
	}
	// sort bill id's
	sort.Slice(dcBillIds, func(i, j int) bool {
		return bytes.Compare(dcBillIds[i], dcBillIds[j]) == -1
	})
	newBillID, billIDs := calcNewBillId(dcBillIds)
	dcTxs, sum := createDCAndSwapTxs(t, newBillID, dcBillIds, txsState)

	dcRecords := make([]*types.TransactionRecord, len(dcTxs))
	dcRecordsProofs := make([]*types.TxProof, len(dcTxs))
	for i, dcTx := range dcTxs {
		err = moneyPrt.SubmitTx(dcTx)
		require.NoError(t, err)
		require.Eventually(t, testpartition.BlockchainContainsTx(moneyPrt, dcTx), test.WaitDuration, test.WaitTick)
		_, dcRecordsProofs[i], dcRecords[i], err = moneyPrt.GetTxProof(dcTx)
		require.NoError(t, err)
	}

	// create swap order
	swapAttr := &SwapDCAttributes{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey1)),
		BillIdentifiers: billIDs,
		DcTransfers:     dcRecords,
		Proofs:          dcRecordsProofs,
		TargetValue:     sum,
	}
	swapBytes, err := cbor.Marshal(swapAttr)
	require.NoError(t, err)
	// create swap tx
	swapTx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   systemIdentifier,
			Type:       PayloadTypeSwapDC,
			UnitID:     newBillID,
			Attributes: swapBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
		FeeProof:   script.PredicateArgumentEmpty(),
	}

	// #nosec G104
	signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey1))
	sigBytes, err := swapTx.PayloadBytes()
	require.NoError(t, err)
	sig, _ := signer.SignBytes(sigBytes)
	swapTx.OwnerProof = script.PredicateArgumentPayToPublicKeyHashDefault(sig, decodeHex(pubKey1))

	err = moneyPrt.SubmitTx(swapTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPrt, swapTx), test.WaitDuration, test.WaitTick)
}

func createSplitTx(t *testing.T, fromID []byte, prevTx *types.TransactionRecord, amount, remaining uint64) *types.TransactionOrder {
	backlink := prevTx.Hash(crypto.SHA256)
	tx, _ := createSplit(t, fromID, amount, remaining, script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey2)), backlink)
	signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey1))
	sigBytes, err := tx.PayloadBytes()
	require.NoError(t, err)
	sig, _ := signer.SignBytes(sigBytes)
	tx.OwnerProof = script.PredicateArgumentPayToPublicKeyHashDefault(sig, decodeHex(pubKey1))
	return tx
}

func createDCAndSwapTxs(
	t *testing.T,
	newBillID []byte,
	ids []types.UnitID, // bills to swap
	s *state.State) ([]*types.TransactionOrder, uint64) {
	t.Helper()

	// create dc transfers
	dcTransfers := make([]*types.TransactionOrder, len(ids))

	var targetValue uint64 = 0
	for i, id := range ids {
		_, billData := getBill(t, s, id)
		// NB! dc transfer nonce must be equal to swap tx unit id
		targetValue += billData.V
		tx, _ := createDCTransfer(t, id, billData.V, billData.Backlink, newBillID, script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey1)))
		signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey2))
		sigBytes, err := tx.PayloadBytes()
		require.NoError(t, err)
		sig, _ := signer.SignBytes(sigBytes)
		tx.OwnerProof = script.PredicateArgumentPayToPublicKeyHashDefault(sig, decodeHex(pubKey2))
		dcTransfers[i] = tx
	}

	return dcTransfers, targetValue
}

func calcNewBillId(ids []types.UnitID) ([]byte, [][]byte) {
	// calculate new bill ID
	hasher := crypto.SHA256.New()
	idsByteArray := make([][]byte, len(ids))
	for i, id := range ids {
		hasher.Write(id)
		idsByteArray[i] = id
	}
	return hasher.Sum(nil), idsByteArray
}

func getBlockProof(t *testing.T, tx *types.TransactionOrder, sysId []byte, network *testpartition.AlphabillNetwork) *types.TxProof {
	partition, err := network.GetNodePartition(sysId)
	require.NoError(t, err)
	// create adapter for conversion interface
	_, proof, _, err := partition.GetTxProof(tx)
	require.NoError(t, err)
	return proof
}

func decodeAndHashHex(hex string) []byte {
	hasher := crypto.SHA256.New()
	hasher.Write(decodeHex(hex))
	return hasher.Sum(nil)
}

func decodeHex(hex string) []byte {
	decoded, _ := hexutil.Decode(hex)
	return decoded
}
