package money

import (
	"crypto"
	"sort"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testmoneyfc "github.com/alphabill-org/alphabill/internal/testutils/money"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	systemIdentifier = []byte{0, 0, 0, 0}

	pubKey1  = "0x0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"
	privKey1 = "0xa5e8bff9733ebc751a45ca4b8cc6ce8e76c8316a5eb556f738092df6232e78de"

	pubKey2  = "0x02d29cbdea6062c0a9d9170245188fa39a12ad3dd6cc02a78fcc026594d9bdc06c"
	privKey2 = "0xd7e5041766e8ca505ab07ffa46652e248ede22b436ec81b583a78c8c9e1aac6b"
)

func TestPartition_Ok(t *testing.T) {
	const moneyInvariant = uint64(10000)
	total := moneyInvariant
	initialBill := &InitialBill{
		ID:    uint256.NewInt(1),
		Value: moneyInvariant,
		Owner: script.PredicateAlwaysTrue(),
	}
	txFee := fc.FixedFee(1)
	network, err := testpartition.NewNetwork(3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
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

	// create fee credit for initial bill transfer
	fcrAmount := testmoneyfc.FCRAmount
	transferFC := testmoneyfc.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)

	// transfer initial bill to pubKey1
	transferInitialBillTx := createBillTransfer(initialBill.ID, total-fcrAmount-txFee(), script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey1)), transferFC.Hash(crypto.SHA256))
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// split initial bill from pubKey1 to pubKey2
	amountPK2 := uint64(1000)
	tx := createSplitTx(transferInitialBillTx, amountPK2, total-fcrAmount-txFee()-amountPK2)
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	// wrong partition tx
	tx = createNonMoneyTx()
	err = network.SubmitTx(tx)
	require.Error(t, err)
	require.Never(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}

func TestPartition_SwapOk(t *testing.T) {
	const moneyInvariant = uint64(10000)
	const nofDustToSwap = 3

	var (
		hashAlgorithm = crypto.SHA256
		state         *rma.Tree
		trustBase     = map[string]abcrypto.Verifier{}
		initialBill   = &InitialBill{
			ID:    uint256.NewInt(1),
			Value: moneyInvariant,
			Owner: script.PredicateAlwaysTrue(),
		}
		feeFunc = fc.FixedFee(1)
	)
	total := moneyInvariant
	network, err := testpartition.NewNetwork(3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		var err error
		state = rma.NewWithSHA256()
		trustBase = tb
		system, err := NewMoneyTxSystem(systemIdentifier,
			WithHashAlgorithm(crypto.SHA256),
			WithInitialBill(initialBill),
			WithSystemDescriptionRecords(createSDRs(2)),
			WithDCMoneyAmount(100),
			WithTrustBase(tb),
			WithState(state),
			WithFeeCalculator(feeFunc),
		)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)

	// create fee credit for initial bill transfer
	txFee := feeFunc()
	fcrAmount := testmoneyfc.FCRAmount
	transferFC := testmoneyfc.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	require.NoError(t, err)

	// transfer initial bill to pubKey1
	transferInitialBillTx := createBillTransfer(initialBill.ID, total-fcrAmount-txFee, script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey1)), transferFC.Hash(crypto.SHA256))
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// split initial bill, create small payments from which to make dust payments
	splitTxs := make([]*txsystem.Transaction, nofDustToSwap)
	amount := uint64(1)
	prev := transferInitialBillTx
	total = total - fcrAmount - txFee
	for i, _ := range splitTxs {
		total = total - amount
		splitTxs[i] = createSplitTx(prev, amount, total)
		amount++
		prev = splitTxs[i]
		err = network.SubmitTx(splitTxs[i])
		require.NoError(t, err)
		// wait for transaction to be added to block
		require.Eventually(t, testpartition.BlockchainContainsTx(splitTxs[i], network), test.WaitDuration, test.WaitTick)
	}

	// create dust payments from splits
	dcBillIds := make([]*uint256.Int, len(splitTxs))
	for i, splitTx := range splitTxs {
		splitGenTx, err := NewMoneyTx(systemIdentifier, splitTx)
		require.NoError(t, err)
		dcBillIds[i] = txutil.SameShardID(splitGenTx.UnitID(), unitIdFromTransaction(splitGenTx.(*billSplitWrapper)))
	}
	// sort bill id's
	sort.Slice(dcBillIds, func(i, j int) bool {
		return dcBillIds[i].Lt(dcBillIds[j])
	})
	newBillID, billIDs := calcNewBillId(dcBillIds)
	dcTxs, sum := createDCAndSwapTxs(t, newBillID, dcBillIds, state)
	for _, dcTx := range dcTxs {
		err = network.SubmitTx(dcTx)
		require.NoError(t, err)
	}
	require.Eventually(t, testpartition.BlockchainContainsTx(dcTxs[len(dcTxs)-1], network), test.WaitDuration, test.WaitTick)

	// create block proofs
	blockProofs := make([]*block.BlockProof, len(dcTxs))
	for i, dcTx := range dcTxs {
		blockProofs[i] = getBlockProof(t, dcTx, systemIdentifier, network)
	}

	// Verify block proofs
	for i, proof := range blockProofs {
		gtx, err := NewMoneyTx(systemIdentifier, dcTxs[i])
		require.NoError(t, err)
		require.NoError(t, proof.Verify(dcTxs[i].UnitId, gtx, trustBase, hashAlgorithm))
	}

	// create swap order
	swapOrder, err := anypb.New(&SwapOrder{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey1)),
		BillIdentifiers: billIDs,
		DcTransfers:     dcTxs,
		Proofs:          blockProofs,
		TargetValue:     sum,
	})
	require.NoError(t, err)

	// create swap tx
	swapTx := &txsystem.Transaction{
		SystemId:              systemIdentifier,
		UnitId:                newBillID,
		TransactionAttributes: swapOrder,
		OwnerProof:            script.PredicateArgumentEmpty(),
		FeeProof:              script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout:           20,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(fcrID),
		},
	}
	// #nosec G104
	gtx, _ := NewMoneyTx(systemIdentifier, swapTx)
	signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey1))
	sig, _ := signer.SignBytes(gtx.SigBytes())
	swapTx.OwnerProof = script.PredicateArgumentPayToPublicKeyHashDefault(sig, decodeHex(pubKey1))

	err = network.SubmitTx(swapTx)
	require.Eventually(t, testpartition.BlockchainContainsTx(swapTx, network), test.WaitDuration, test.WaitTick)
}

func createSplitTx(prevTx *txsystem.Transaction, amount, remaining uint64) *txsystem.Transaction {
	backlinkTx, _ := NewMoneyTx(systemIdentifier, prevTx)
	backlink := backlinkTx.Hash(crypto.SHA256)

	tx := createSplit(uint256.NewInt(1), amount, remaining, script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey2)), backlink)
	gtx, _ := NewMoneyTx(systemIdentifier, tx)
	signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey1))
	sig, _ := signer.SignBytes(gtx.SigBytes())
	tx.OwnerProof = script.PredicateArgumentPayToPublicKeyHashDefault(sig, decodeHex(pubKey1))
	return tx
}

func createDCAndSwapTxs(
	t *testing.T,
	newBillID []byte,
	ids []*uint256.Int, // bills to swap
	rmaTree *rma.Tree) ([]*txsystem.Transaction, uint64) {
	t.Helper()

	// create dc transfers
	dcTransfers := make([]*txsystem.Transaction, len(ids))

	var targetValue uint64 = 0
	for i, id := range ids {
		_, billData := getBill(t, rmaTree, id)
		// NB! dc transfer nonce must be equal to swap tx unit id
		targetValue += billData.V
		tx := createDCTransfer(id, billData.V, billData.Backlink, newBillID, script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey1)))
		gtx, _ := NewMoneyTx(systemIdentifier, tx)
		signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey2))
		sig, _ := signer.SignBytes(gtx.SigBytes())
		tx.OwnerProof = script.PredicateArgumentPayToPublicKeyHashDefault(sig, decodeHex(pubKey2))
		dcTransfers[i] = tx
	}

	return dcTransfers, targetValue
}

func calcNewBillId(ids []*uint256.Int) ([]byte, [][]byte) {
	// calculate new bill ID
	hasher := crypto.SHA256.New()
	idsByteArray := make([][]byte, len(ids))
	for i, id := range ids {
		bytes32 := id.Bytes32()
		hasher.Write(bytes32[:])
		idsByteArray[i] = bytes32[:]
	}
	return hasher.Sum(nil), idsByteArray
}

func getBlockProof(t *testing.T, tx *txsystem.Transaction, sysId []byte, network *testpartition.AlphabillPartition) *block.BlockProof {
	// create adapter for conversion interface
	txConverter := func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
		return NewMoneyTx(sysId, tx)
	}
	_, ttt, err := network.GetBlockProof(tx, txConverter)
	require.NoError(t, err)
	return ttt
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

func createFeeCredit(t *testing.T, initialBill *InitialBill, network *testpartition.AlphabillPartition) (uint64, *transactions.TransferFeeCreditWrapper) {
	// send transferFC
	fcrIDBytes := fcrID.Bytes32()
	fcrAmount := uint64(100)
	transferFC := testfc.NewTransferFC(t,
		testfc.NewTransferFCAttr(
			testfc.WithBacklink(nil),
			testfc.WithAmount(fcrAmount),
			testfc.WithTargetRecordID(fcrIDBytes[:]),
		),
		testtransaction.WithUnitId(util.Uint256ToBytes(initialBill.ID)),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
	require.NoError(t, network.SubmitTx(transferFC.Transaction))
	require.Eventually(t, testpartition.BlockchainContainsTx(transferFC.Transaction, network), test.WaitDuration, test.WaitTick)

	// send addFC
	_, transferFCProof, err := network.GetBlockProof(transferFC.Transaction, transactions.NewFeeCreditTx)
	require.NoError(t, err)
	addFC := testfc.NewAddFC(t, network.RootSigner,
		testfc.NewAddFCAttr(t, network.RootSigner,
			testfc.WithTransferFCTx(transferFC.Transaction),
			testfc.WithTransferFCProof(transferFCProof),
			testfc.WithFCOwnerCondition(script.PredicateAlwaysTrue()),
		),
		testtransaction.WithUnitId(fcrIDBytes[:]),
	)
	require.NoError(t, network.SubmitTx(addFC.Transaction))
	require.Eventually(t, testpartition.BlockchainContainsTx(addFC.Transaction, network), test.WaitDuration, test.WaitTick)
	return fcrAmount, transferFC
}
