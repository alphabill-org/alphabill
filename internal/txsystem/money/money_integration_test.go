package money

import (
	"crypto"
	"fmt"
	"testing"

	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testpartition "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
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
	network, err := testpartition.NewNetwork(3, func() txsystem.TransactionSystem {
		system, err := NewMoneyTxSystem(
			crypto.SHA256,
			&InitialBill{
				ID:    uint256.NewInt(1),
				Value: 10000,
				Owner: script.PredicateAlwaysTrue(),
			},
			0,
		)
		require.NoError(t, err)
		return system
	}, systemIdentifier)
	require.NoError(t, err)

	// transfer initial bill to pubKey1
	transferInitialBillTx := createBillTransfer(uint256.NewInt(1), 10000, script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey1)), nil)
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", transferInitialBillTx, transferInitialBillTx.UnitId)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// split initial bill from pubKey1 to pubKey2
	tx := createSplitTx(transferInitialBillTx)
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = network.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	// wrong partition tx
	tx = createNonMoneyTx()
	err = network.SubmitTx(tx)
	require.Error(t, err)
	require.Never(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}

func createSplitTx(prevTx *txsystem.Transaction) *txsystem.Transaction {
	backlinkTx, _ := NewMoneyTx(systemIdentifier, prevTx)
	backlink := backlinkTx.Hash(crypto.SHA256)

	tx := createSplit(uint256.NewInt(1), 1000, 9000, script.PredicatePayToPublicKeyHashDefault(decodeAndHashHex(pubKey2)), backlink)
	gtx, _ := NewMoneyTx(systemIdentifier, tx)
	signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey1))
	sig, _ := signer.SignBytes(gtx.SigBytes())
	tx.OwnerProof = script.PredicateArgumentPayToPublicKeyHashDefault(sig, decodeHex(pubKey1))
	return tx
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
