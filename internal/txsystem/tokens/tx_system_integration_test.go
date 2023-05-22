package tokens

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var feeCreditID = uint256.NewInt(420)
var defaultClientMetadata = &types.ClientMetadata{
	Timeout:           20,
	MaxTransactionFee: 10,
	FeeCreditRecordID: util.Uint256ToBytes(feeCreditID),
}

func TestInitPartitionAndCreateNFTType_Ok(t *testing.T) {
	tokenPrt, err := testpartition.NewPartition(3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := New(WithTrustBase(trustBase), WithState(newStateWithFeeCredit(t, feeCreditID)))
		require.NoError(t, err)
		return system
	}, DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{tokenPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })

	tx := testtransaction.NewTransaction(t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId([]byte{0, 0, 0, 1}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                   "Test",
				Name:                     "Long name for Test",
				Icon:                     &Icon{Type: validIconType, Data: []byte{3, 2, 1}},
				ParentTypeID:             util.Uint256ToBytes(uint256.NewInt(0)),
				SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
				TokenCreationPredicate:   script.PredicateAlwaysTrue(),
				InvariantPredicate:       script.PredicateAlwaysTrue(),
				DataUpdatePredicate:      script.PredicateAlwaysTrue(),
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(tx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, tx), test.WaitDuration, test.WaitTick)
}

func TestFungibleTokenTransactions_Ok(t *testing.T) {
	var (
		hashAlgorithm       = gocrypto.SHA256
		states              []*rma.Tree
		zeroID                     = util.Uint256ToBytes(uint256.NewInt(0))
		fungibleTokenTypeID        = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		fungibleTokenID1           = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
		totalValue          uint64 = 1000
		splitValue1         uint64 = 100
		splitValue2         uint64 = 10
		trustBase                  = map[string]crypto.Verifier{}
	)

	// setup network
	tokenPrt, err := testpartition.NewPartition(1, func(tb map[string]crypto.Verifier) txsystem.TransactionSystem {
		trustBase = tb
		state := newStateWithFeeCredit(t, feeCreditID)
		system, err := New(WithState(state), WithTrustBase(tb))
		require.NoError(t, err)
		states = append(states, state)
		return system
	}, DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	// the tx system lambda is called once for node genesis, but this is not interesting so clear the states before node
	// is started
	states = []*rma.Tree{}
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{tokenPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })
	state := states[0]

	// create fungible token type
	createTypeTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenTypeID),
		testtransaction.WithPayloadType(PayloadTypeCreateFungibleTokenType),
		testtransaction.WithAttributes(
			&CreateFungibleTokenTypeAttributes{
				Symbol:                   "ALPHA",
				Name:                     "Long name for ALPHA",
				Icon:                     &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
				ParentTypeID:             zeroID,
				SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
				TokenCreationPredicate:   script.PredicateAlwaysTrue(),
				InvariantPredicate:       script.PredicateAlwaysTrue(),
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(createTypeTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, createTypeTx), test.WaitDuration*4, test.WaitTick)
	RequireFungibleTokenTypeState(t, state, fungibleTokenTypeUnitData{
		tokenCreationPredicate:   script.PredicateAlwaysTrue(),
		subTypeCreationPredicate: script.PredicateAlwaysTrue(),
		invariantPredicate:       script.PredicateAlwaysTrue(),
		unitID:                   fungibleTokenTypeID,
		bearer:                   script.PredicateAlwaysTrue(),
		symbol:                   "ALPHA",
		name:                     "Long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		parentID:                 zeroID,
		decimalPlaces:            0,
	})
	verifyProof(t, createTypeTx, tokenPrt, trustBase, hashAlgorithm)

	// mint token
	mintTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithPayloadType(PayloadTypeMintFungibleToken),
		testtransaction.WithAttributes(
			&MintFungibleTokenAttributes{
				Bearer:                           script.PredicateAlwaysTrue(),
				Type:                             fungibleTokenTypeID,
				Value:                            totalValue,
				TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(mintTx))
	txHash := mintTx.Hash(gocrypto.SHA256)
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, mintTx), test.WaitDuration*4, test.WaitTick)
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   txHash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue,
	})
	verifyProof(t, mintTx, tokenPrt, trustBase, hashAlgorithm)

	// split token
	splitTx1 := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeSplitFungibleToken),
		testtransaction.WithAttributes(
			&SplitFungibleTokenAttributes{
				Type:                         fungibleTokenTypeID,
				NewBearer:                    script.PredicateAlwaysTrue(),
				TargetValue:                  splitValue1,
				RemainingValue:               totalValue - splitValue1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     txHash,
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(splitTx1))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, splitTx1), test.WaitDuration, test.WaitTick)

	split1GenTxHash := splitTx1.Hash(hashAlgorithm)
	require.NoError(t, err)
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   split1GenTxHash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue - splitValue1,
	})
	verifyProof(t, splitTx1, tokenPrt, trustBase, hashAlgorithm)

	sUnitID1 := txutil.SameShardID(uint256.NewInt(0).SetBytes(fungibleTokenID1), hashForIDCalculation(splitTx1, hashAlgorithm))
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     util.Uint256ToBytes(sUnitID1),
		typeUnitID: fungibleTokenTypeID,
		backlink:   split1GenTxHash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: splitValue1,
	})

	splitTx2 := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeSplitFungibleToken),
		testtransaction.WithAttributes(
			&SplitFungibleTokenAttributes{
				Type:                         fungibleTokenTypeID,
				NewBearer:                    script.PredicateAlwaysTrue(),
				TargetValue:                  splitValue2,
				RemainingValue:               totalValue - (splitValue1 + splitValue2),
				Nonce:                        nil,
				Backlink:                     splitTx1.Hash(hashAlgorithm),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(splitTx2))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, splitTx2), test.WaitDuration, test.WaitTick)
	verifyProof(t, splitTx2, tokenPrt, trustBase, hashAlgorithm)

	splitGenTx2Hash := splitTx2.Hash(hashAlgorithm)
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   splitGenTx2Hash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	sUnitID2 := txutil.SameShardID(uint256.NewInt(0).SetBytes(fungibleTokenID1), hashForIDCalculation(splitTx2, hashAlgorithm))
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     util.Uint256ToBytes(sUnitID2),
		typeUnitID: fungibleTokenTypeID,
		backlink:   splitGenTx2Hash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: splitValue2,
	})

	// Transfer token
	transferTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeTransferFungibleToken),
		testtransaction.WithAttributes(
			&TransferFungibleTokenAttributes{
				Type:                         fungibleTokenTypeID,
				NewBearer:                    script.PredicateAlwaysTrue(),
				Value:                        totalValue - splitValue1 - splitValue2,
				Nonce:                        nil,
				Backlink:                     splitTx2.Hash(hashAlgorithm),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(transferTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, transferTx), test.WaitDuration, test.WaitTick)
	verifyProof(t, transferTx, tokenPrt, trustBase, hashAlgorithm)
	transferGenTxHash := transferTx.Hash(hashAlgorithm)
	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   transferGenTxHash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	// burn token x 2
	burnTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(util.Uint256ToBytes(sUnitID1)),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeBurnFungibleToken),
		testtransaction.WithAttributes(
			&BurnFungibleTokenAttributes{
				Type:                         fungibleTokenTypeID,
				Value:                        splitValue1,
				Nonce:                        transferTx.Hash(hashAlgorithm),
				Backlink:                     split1GenTxHash,
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(burnTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, burnTx), test.WaitDuration, test.WaitTick)
	verifyProof(t, burnTx, tokenPrt, trustBase, hashAlgorithm)

	burnTx2 := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(util.Uint256ToBytes(sUnitID2)),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeBurnFungibleToken),
		testtransaction.WithAttributes(
			&BurnFungibleTokenAttributes{
				Type:                         fungibleTokenTypeID,
				Value:                        splitValue2,
				Nonce:                        transferTx.Hash(hashAlgorithm),
				Backlink:                     splitGenTx2Hash,
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(burnTx2))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, burnTx2), test.WaitDuration, test.WaitTick)
	verifyProof(t, burnTx2, tokenPrt, trustBase, hashAlgorithm)

	// join token
	_, burnProof1, burnTxRecord, err := tokenPrt.GetBlockProof(burnTx)
	require.NoError(t, err)

	require.NoError(t, types.VerifyTxProof(burnProof1, burnTxRecord, trustBase, hashAlgorithm))

	_, burnProof2, burnTxRecord2, err := tokenPrt.GetBlockProof(burnTx2)
	require.NoError(t, err)
	require.NoError(t, types.VerifyTxProof(burnProof2, burnTxRecord2, trustBase, hashAlgorithm))
	joinTx := testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeJoinFungibleToken),
		testtransaction.WithAttributes(
			&JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTxRecord, burnTxRecord2},
				Proofs:                       []*types.TxProof{burnProof1, burnProof2},
				Backlink:                     transferTx.Hash(hashAlgorithm),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(joinTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, joinTx), test.WaitDuration, test.WaitTick)
	u, err := states[0].GetUnit(uint256.NewInt(0).SetBytes(fungibleTokenID1))
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data)
	d := u.Data.(*fungibleTokenData)
	require.NotNil(t, totalValue, d.value)

	RequireFungibleTokenState(t, state, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   joinTx.Hash(hashAlgorithm),
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue,
	})

	unit, err := state.GetUnit(feeCreditID)
	require.NoError(t, err)
	require.Equal(t, uint64(92), unit.Data.(*fc.FeeCreditRecord).Balance)
}

func verifyProof(t *testing.T, tx *types.TransactionOrder, prt *testpartition.NodePartition, trustBase map[string]crypto.Verifier, hashAlgorithm gocrypto.Hash) {
	_, proof, record, err := prt.GetBlockProof(tx)
	require.NoError(t, err)
	require.NoError(t, types.VerifyTxProof(proof, record, trustBase, hashAlgorithm))
}

type fungibleTokenUnitData struct {
	unitID, typeUnitID, backlink, bearer []byte
	tokenValue                           uint64
}

type fungibleTokenTypeUnitData struct {
	parentID, unitID, bearer                                             []byte
	symbol, name                                                         string
	icon                                                                 *Icon
	decimalPlaces                                                        uint32
	tokenCreationPredicate, subTypeCreationPredicate, invariantPredicate []byte
}

func RequireFungibleTokenTypeState(t *testing.T, state *rma.Tree, e fungibleTokenTypeUnitData) {
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
	require.Equal(t, e.name, d.name)
	require.Equal(t, e.icon.Type, d.icon.Type)
	require.Equal(t, e.icon.Data, d.icon.Data)
	require.Equal(t, uint256.NewInt(0).SetBytes(e.parentID), d.parentTypeId)
	require.Equal(t, e.decimalPlaces, d.decimalPlaces)
}

func RequireFungibleTokenState(t *testing.T, state *rma.Tree, e fungibleTokenUnitData) {
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

func newStateWithFeeCredit(t *testing.T, feeCreditID *uint256.Int) *rma.Tree {
	state := rma.NewWithSHA256()
	require.NoError(t, state.AtomicUpdate(
		fc.AddCredit(feeCreditID, script.PredicateAlwaysTrue(), &fc.FeeCreditRecord{
			Balance: 100,
			Hash:    make([]byte, 32),
			Timeout: 1000,
		}, make([]byte, 32)),
	))
	state.Commit()
	return state
}
