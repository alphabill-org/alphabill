package testutils

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"

	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

// CreateFeeCredit creates fee credit to be able to spend initial bill
func CreateFeeCredit(t *testing.T, initialBillID, fcrID types.UnitID, fcrAmount uint64, privKey []byte, pubKey []byte, network *testpartition.AlphabillNetwork) *types.TransactionOrder {
	// send transferFC
	transferFC := NewTransferFC(t,
		NewTransferFCAttr(
			WithBacklink(nil),
			WithAmount(fcrAmount),
			WithTargetRecordID(fcrID),
		),
		testtransaction.WithUnitId(initialBillID),
		testtransaction.WithPayloadType(transactions.PayloadTypeTransferFeeCredit),
	)

	signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(privKey)
	sigBytes, err := transferFC.PayloadBytes()
	require.NoError(t, err)
	sig, _ := signer.SignBytes(sigBytes)
	transferFC.OwnerProof = templates.NewP2pkh256SignatureBytes(sig, pubKey)

	moneyPartition, err := network.GetNodePartition([]byte{0, 0, 0, 0})
	require.NoError(t, err)
	require.NoError(t, moneyPartition.SubmitTx(transferFC))

	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPartition, testpartition.ANY_VALIDATOR, transferFC)
	require.NoError(t, err, "transfer fee credit tx failed")
	// send addFC
	addFC := NewAddFC(t, network.RootPartition.Nodes[0].RootSigner,
		NewAddFCAttr(t, network.RootPartition.Nodes[0].RootSigner,
			WithTransferFCTx(transferFCRecord),
			WithTransferFCProof(transferFCProof),
			WithFCOwnerCondition(templates.NewP2pkh256BytesFromKey(pubKey)),
		),
		testtransaction.WithUnitId(fcrID),
		testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKey(pubKey)),
		testtransaction.WithPayloadType(transactions.PayloadTypeAddFeeCredit),
	)
	require.NoError(t, moneyPartition.SubmitTx(addFC))
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, addFC), test.WaitDuration, test.WaitTick)
	return transferFCRecord.TransactionOrder
}
