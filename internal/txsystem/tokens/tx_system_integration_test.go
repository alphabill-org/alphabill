package tokens

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestInitPartitionAndCreateNFTType_Ok(t *testing.T) {
	network, err := testpartition.NewNetwork(3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := New(WithTrustBase(trustBase))
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
				ParentTypeId:             util.Uint256ToBytes(uint256.NewInt(0)),
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

func TestFungibleTokenTransactions_Ok(t *testing.T) {
	var (
		hashAlgorithm       = gocrypto.SHA256
		states              []TokenState
		zeroID                     = util.Uint256ToBytes(uint256.NewInt(0))
		fungibleTokenTypeID        = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		fungibleTokenID1           = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
		totalValue          uint64 = 1000
		splitValue1         uint64 = 100
		splitValue2         uint64 = 10
		zeroBacklink               = make([]byte, 32)
		trustBase                  = map[string]crypto.Verifier{}
	)

	// setup network
	network, err := testpartition.NewNetwork(3, func(tb map[string]crypto.Verifier) txsystem.TransactionSystem {
		trustBase = tb
		state, err := rma.New(&rma.Config{
			HashAlgorithm: hashAlgorithm,
		})
		require.NoError(t, err)
		system, err := New(WithState(state), WithTrustBase(tb))
		require.NoError(t, err)
		states = append(states, state)
		return system
	}, DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	require.NotNil(t, network)
	state := states[0]

	// create fungible token type
	createTypeTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenTypeID),
		testtransaction.WithAttributes(
			&CreateFungibleTokenTypeAttributes{
				Symbol:                   "ALPHA",
				ParentTypeId:             zeroID,
				SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
				TokenCreationPredicate:   script.PredicateAlwaysTrue(),
				InvariantPredicate:       script.PredicateAlwaysTrue(),
			},
		),
	)
	require.NoError(t, network.BroadcastTx(createTypeTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(createTypeTx, network), test.WaitDuration*4, test.WaitTick)
	RequireFungibleTokenTypeState(t, state, fungibleTokenTypeUnitData{
		tokenCreationPredicate:   script.PredicateAlwaysTrue(),
		subTypeCreationPredicate: script.PredicateAlwaysTrue(),
		invariantPredicate:       script.PredicateAlwaysTrue(),
		unitID:                   fungibleTokenTypeID,
		bearer:                   script.PredicateAlwaysTrue(),
		symbol:                   "ALPHA",
		parentID:                 zeroID,
		decimalPlaces:            0,
	})
	verifyProof(t, createTypeTx, network, trustBase, hashAlgorithm)

	// mint token
	mintTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithAttributes(
			&MintFungibleTokenAttributes{
				Bearer:                           script.PredicateAlwaysTrue(),
				Type:                             fungibleTokenTypeID,
				Value:                            totalValue,
				TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
	)
	require.NoError(t, network.BroadcastTx(mintTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(mintTx, network), test.WaitDuration*4, test.WaitTick)
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   zeroBacklink,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue,
	})
	verifyProof(t, mintTx, network, trustBase, hashAlgorithm)

	// split token
	splitTx1 := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithAttributes(
			&SplitFungibleTokenAttributes{
				NewBearer:                   script.PredicateAlwaysTrue(),
				TargetValue:                 splitValue1,
				Nonce:                       test.RandomBytes(32),
				Backlink:                    make([]byte, 32),
				InvariantPredicateSignature: script.PredicateArgumentEmpty(),
			},
		),
	)
	require.NoError(t, network.BroadcastTx(splitTx1))
	require.Eventually(t, testpartition.BlockchainContainsTx(splitTx1, network), test.WaitDuration, test.WaitTick)
	split1GenTx, err := NewGenericTx(splitTx1)
	require.NoError(t, err)
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   split1GenTx.Hash(hashAlgorithm),
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue - splitValue1,
	})
	verifyProof(t, splitTx1, network, trustBase, hashAlgorithm)

	sUnitID1 := txutil.SameShardID(uint256.NewInt(0).SetBytes(fungibleTokenID1), split1GenTx.(*splitFungibleTokenWrapper).HashForIDCalculation(hashAlgorithm))
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     util.Uint256ToBytes(sUnitID1),
		typeUnitID: fungibleTokenTypeID,
		backlink:   zeroBacklink,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: splitValue1,
	})

	splitTx2 := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithAttributes(
			&SplitFungibleTokenAttributes{
				NewBearer:                   script.PredicateAlwaysTrue(),
				TargetValue:                 splitValue2,
				Nonce:                       nil,
				Backlink:                    split1GenTx.Hash(hashAlgorithm),
				InvariantPredicateSignature: script.PredicateArgumentEmpty(),
			},
		),
	)
	require.NoError(t, network.BroadcastTx(splitTx2))
	require.Eventually(t, testpartition.BlockchainContainsTx(splitTx2, network), test.WaitDuration, test.WaitTick)
	verifyProof(t, splitTx2, network, trustBase, hashAlgorithm)
	splitGenTx2, err := NewGenericTx(splitTx2)
	require.NoError(t, err)
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   splitGenTx2.Hash(hashAlgorithm),
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	sUnitID2 := txutil.SameShardID(uint256.NewInt(0).SetBytes(fungibleTokenID1), splitGenTx2.(*splitFungibleTokenWrapper).HashForIDCalculation(hashAlgorithm))
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     util.Uint256ToBytes(sUnitID2),
		typeUnitID: fungibleTokenTypeID,
		backlink:   zeroBacklink,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: splitValue2,
	})

	// Transfer token
	transferTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithAttributes(
			&TransferFungibleTokenAttributes{
				NewBearer:                   script.PredicateAlwaysTrue(),
				Value:                       totalValue - splitValue1 - splitValue2,
				Nonce:                       nil,
				Backlink:                    splitGenTx2.Hash(hashAlgorithm),
				InvariantPredicateSignature: script.PredicateArgumentEmpty(),
			},
		),
	)
	require.NoError(t, network.BroadcastTx(transferTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(transferTx, network), test.WaitDuration, test.WaitTick)
	verifyProof(t, transferTx, network, trustBase, hashAlgorithm)
	transferGenTx, err := NewGenericTx(transferTx)
	require.NoError(t, err)
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   transferGenTx.Hash(hashAlgorithm),
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	// burn token x 2
	burnTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(util.Uint256ToBytes(sUnitID1)),
		testtransaction.WithAttributes(
			&BurnFungibleTokenAttributes{
				Type:                        fungibleTokenTypeID,
				Value:                       splitValue1,
				Nonce:                       transferGenTx.Hash(hashAlgorithm),
				Backlink:                    make([]byte, 32),
				InvariantPredicateSignature: script.PredicateArgumentEmpty(),
			},
		),
	)
	require.NoError(t, network.BroadcastTx(burnTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(burnTx, network), test.WaitDuration, test.WaitTick)
	verifyProof(t, burnTx, network, trustBase, hashAlgorithm)
	burnGenTx, err := NewGenericTx(burnTx)
	require.NoError(t, err)

	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     util.Uint256ToBytes(sUnitID1),
		typeUnitID: fungibleTokenTypeID,
		backlink:   burnGenTx.Hash(hashAlgorithm),
		bearer:     []byte{0},
		tokenValue: splitValue1,
	})

	burnTx2 := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(util.Uint256ToBytes(sUnitID2)),
		testtransaction.WithAttributes(
			&BurnFungibleTokenAttributes{
				Type:                        fungibleTokenTypeID,
				Value:                       splitValue2,
				Nonce:                       transferGenTx.Hash(hashAlgorithm),
				Backlink:                    make([]byte, 32),
				InvariantPredicateSignature: script.PredicateArgumentEmpty(),
			},
		),
	)
	require.NoError(t, network.BroadcastTx(burnTx2))
	require.Eventually(t, testpartition.BlockchainContainsTx(burnTx2, network), test.WaitDuration, test.WaitTick)
	verifyProof(t, burnTx2, network, trustBase, hashAlgorithm)
	burnGenTx2, err := NewGenericTx(burnTx2)
	require.NoError(t, err)

	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     util.Uint256ToBytes(sUnitID2),
		typeUnitID: fungibleTokenTypeID,
		backlink:   burnGenTx2.Hash(hashAlgorithm),
		bearer:     []byte{0},
		tokenValue: splitValue2,
	})

	// join token
	_, burnProof1, err := network.GetBlockProof(burnTx, NewGenericTx)
	require.NoError(t, err)

	require.NoError(t, burnProof1.Verify(burnGenTx, trustBase, hashAlgorithm))

	_, burnProof2, err := network.GetBlockProof(burnTx2, NewGenericTx)
	require.NoError(t, err)
	require.NoError(t, burnProof2.Verify(burnGenTx2, trustBase, hashAlgorithm))
	joinTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithAttributes(
			&JoinFungibleTokenAttributes{
				BurnTransactions:            []*txsystem.Transaction{burnTx, burnTx2},
				Proofs:                      []*block.BlockProof{burnProof1, burnProof2},
				Backlink:                    transferGenTx.Hash(hashAlgorithm),
				InvariantPredicateSignature: script.PredicateArgumentEmpty(),
			},
		),
	)
	require.NoError(t, network.BroadcastTx(joinTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(joinTx, network), test.WaitDuration, test.WaitTick)
	u, err := states[0].GetUnit(uint256.NewInt(0).SetBytes(fungibleTokenID1))
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data)
	d := u.Data.(*fungibleTokenData)
	require.NotNil(t, totalValue, d.value)

	joinGenTx, err := NewGenericTx(joinTx)
	require.NoError(t, err)

	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   joinGenTx.Hash(hashAlgorithm),
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue,
	})
}

func verifyProof(t *testing.T, tx *txsystem.Transaction, network *testpartition.AlphabillPartition, trustBase map[string]crypto.Verifier, hashAlgorithm gocrypto.Hash) {
	gtx, err := NewGenericTx(tx)
	require.NoError(t, err)
	_, ttt, err := network.GetBlockProof(tx, NewGenericTx)
	require.NoError(t, err)
	require.NoError(t, ttt.Verify(gtx, trustBase, hashAlgorithm))
}

type fungibleTokenUnitData struct {
	unitID, typeUnitID, backlink, bearer []byte
	tokenValue                           uint64
}

type fungibleTokenTypeUnitData struct {
	parentID, unitID, bearer                                             []byte
	symbol                                                               string
	decimalPlaces                                                        uint32
	tokenCreationPredicate, subTypeCreationPredicate, invariantPredicate []byte
}

func RequireFungibleTokenTypeState(t *testing.T, state TokenState, e fungibleTokenTypeUnitData) {
	t.Helper()
	u, err := state.GetUnit(uint256.NewInt(0).SetBytes(e.unitID))
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, e.bearer, []byte(u.Bearer))
	require.IsType(t, &fungibleTokenTypeData{}, u.Data)
	d := u.Data.(*fungibleTokenTypeData)
	require.Equal(t, e.tokenCreationPredicate, d.tokenCreationPredicate)
	require.Equal(t, e.subTypeCreationPredicate, d.subTypeCreationPredicate)
	require.Equal(t, e.invariantPredicate, d.invariantPredicate)
	require.Equal(t, e.symbol, d.symbol)
	require.Equal(t, uint256.NewInt(0).SetBytes(e.parentID), d.parentTypeId)
	require.Equal(t, e.decimalPlaces, d.decimalPlaces)
}

func RequireFungibleTokenState(t *testing.T, state TokenState, e fungibleTokenUnitData) {
	t.Helper()
	u, err := state.GetUnit(uint256.NewInt(0).SetBytes(e.unitID))
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, e.bearer, []byte(u.Bearer))
	require.IsType(t, &fungibleTokenData{}, u.Data)
	d := u.Data.(*fungibleTokenData)
	require.Equal(t, e.tokenValue, d.value)
	require.Equal(t, e.backlink, d.backlink)
	require.Equal(t, uint256.NewInt(0).SetBytes(e.typeUnitID), d.tokenType)
}
