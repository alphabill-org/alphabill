package tokens

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestInitPartitionAndCreateNFTType_Ok(t *testing.T) {
	network, err := testpartition.NewNetwork(3, func() txsystem.TransactionSystem {
		system, err := New()
		require.NoError(t, err)
		return system
	}, DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	require.NotNil(t, network)
	tx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId([]byte{0, 0, 0, 1}),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                   "Test",
				ParentTypeId:             uint256.NewInt(0).Bytes(),
				SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
				TokenCreationPredicate:   script.PredicateAlwaysTrue(),
				InvariantPredicate:       script.PredicateAlwaysTrue(),
				DataUpdatePredicate:      script.PredicateAlwaysTrue(),
			},
		),
	)
	require.NoError(t, network.BroadcastTx(tx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}
