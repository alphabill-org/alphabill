package tokens

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	fcunit "github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

var feeCreditID = NewFeeCreditRecordID(nil, []byte{42})
var defaultClientMetadata = &types.ClientMetadata{
	Timeout:           20,
	MaxTransactionFee: 10,
	FeeCreditRecordID: feeCreditID,
}

func TestInitPartitionAndCreateNFTType_Ok(t *testing.T) {
	tokenPrt, err := testpartition.NewPartition(3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := NewTxSystem(WithTrustBase(trustBase), WithState(newStateWithFeeCredit(t, feeCreditID)))
		require.NoError(t, err)
		return system
	}, DefaultSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{tokenPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })

	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(NewNonFungibleTokenTypeID(nil, []byte{1})),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                   "Test",
				Name:                     "Long name for Test",
				Icon:                     &Icon{Type: validIconType, Data: []byte{3, 2, 1}},
				ParentTypeID:             nil,
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
		states              []*state.State
		fungibleTokenTypeID        = NewFungibleTokenTypeID(nil, []byte{1})
		fungibleTokenID1           = NewFungibleTokenID(nil, []byte{2})
		totalValue          uint64 = 1000
		splitValue1         uint64 = 100
		splitValue2         uint64 = 10
		trustBase                  = map[string]crypto.Verifier{}
	)

	// setup network
	tokenPrt, err := testpartition.NewPartition(1, func(tb map[string]crypto.Verifier) txsystem.TransactionSystem {
		trustBase = tb
		s := newStateWithFeeCredit(t, feeCreditID)
		system, err := NewTxSystem(WithState(s), WithTrustBase(tb))
		require.NoError(t, err)
		states = append(states, s)
		return system
	}, DefaultSystemIdentifier)
	require.NoError(t, err)
	// the tx system lambda is called once for node genesis, but this is not interesting so clear the states before node
	// is started
	states = []*state.State{}
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{tokenPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })
	state0 := states[0]

	// create fungible token type
	createTypeTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenTypeID),
		testtransaction.WithPayloadType(PayloadTypeCreateFungibleTokenType),
		testtransaction.WithAttributes(
			&CreateFungibleTokenTypeAttributes{
				Symbol:                   "ALPHA",
				Name:                     "Long name for ALPHA",
				Icon:                     &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
				ParentTypeID:             nil,
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
	RequireFungibleTokenTypeState(t, state0, fungibleTokenTypeUnitData{
		tokenCreationPredicate:   script.PredicateAlwaysTrue(),
		subTypeCreationPredicate: script.PredicateAlwaysTrue(),
		invariantPredicate:       script.PredicateAlwaysTrue(),
		unitID:                   fungibleTokenTypeID,
		bearer:                   script.PredicateAlwaysTrue(),
		symbol:                   "ALPHA",
		name:                     "Long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		parentID:                 nil,
		decimalPlaces:            0,
	})
	verifyProof(t, createTypeTx, tokenPrt, trustBase, hashAlgorithm)

	// mint token
	mintTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithPayloadType(PayloadTypeMintFungibleToken),
		testtransaction.WithAttributes(
			&MintFungibleTokenAttributes{
				Bearer:                           script.PredicateAlwaysTrue(),
				TypeID:                           fungibleTokenTypeID,
				Value:                            totalValue,
				TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(mintTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, mintTx), test.WaitDuration*4, test.WaitTick)

	_, _, mintTXR, err := tokenPrt.GetTxProof(mintTx)
	require.NoError(t, err)
	txHash := mintTXR.TransactionOrder.Hash(gocrypto.SHA256)

	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   txHash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue,
	})
	verifyProof(t, mintTx, tokenPrt, trustBase, hashAlgorithm)

	// split token
	splitTx1 := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeSplitFungibleToken),
		testtransaction.WithAttributes(
			&SplitFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
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

	_, _, split1GenTXR, err := tokenPrt.GetTxProof(splitTx1)
	require.NoError(t, err)
	split1GenTxHash := split1GenTXR.TransactionOrder.Hash(gocrypto.SHA256)

	require.NoError(t, err)
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   split1GenTxHash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue - splitValue1,
	})
	verifyProof(t, splitTx1, tokenPrt, trustBase, hashAlgorithm)

	sUnitID1 := NewFungibleTokenID(fungibleTokenID1, HashForIDCalculation(splitTx1, hashAlgorithm))
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     sUnitID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   split1GenTxHash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: splitValue1,
	})

	splitTx2 := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeSplitFungibleToken),
		testtransaction.WithAttributes(
			&SplitFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
				NewBearer:                    script.PredicateAlwaysTrue(),
				TargetValue:                  splitValue2,
				RemainingValue:               totalValue - (splitValue1 + splitValue2),
				Nonce:                        nil,
				Backlink:                     split1GenTXR.TransactionOrder.Hash(hashAlgorithm),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(splitTx2))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, splitTx2), test.WaitDuration, test.WaitTick)
	verifyProof(t, splitTx2, tokenPrt, trustBase, hashAlgorithm)

	_, _, split2GenTXR, err := tokenPrt.GetTxProof(splitTx2)
	require.NoError(t, err)
	splitGenTx2Hash := split2GenTXR.TransactionOrder.Hash(gocrypto.SHA256)

	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   splitGenTx2Hash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	sUnitID2 := NewFungibleTokenID(fungibleTokenID1, HashForIDCalculation(splitTx2, hashAlgorithm))
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     sUnitID2,
		typeUnitID: fungibleTokenTypeID,
		backlink:   splitGenTx2Hash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: splitValue2,
	})

	// Transfer token
	transferTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeTransferFungibleToken),
		testtransaction.WithAttributes(
			&TransferFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
				NewBearer:                    script.PredicateAlwaysTrue(),
				Value:                        totalValue - splitValue1 - splitValue2,
				Nonce:                        nil,
				Backlink:                     splitGenTx2Hash,
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(transferTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, transferTx), test.WaitDuration, test.WaitTick)
	verifyProof(t, transferTx, tokenPrt, trustBase, hashAlgorithm)

	_, _, transferTXR, err := tokenPrt.GetTxProof(transferTx)
	require.NoError(t, err)
	transferGenTxHash := transferTXR.TransactionOrder.Hash(gocrypto.SHA256)

	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   transferGenTxHash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	// burn token x 2
	burnTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(sUnitID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeBurnFungibleToken),
		testtransaction.WithAttributes(
			&BurnFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
				Value:                        splitValue1,
				Nonce:                        transferGenTxHash,
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

	burnTx2 := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(sUnitID2),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeBurnFungibleToken),
		testtransaction.WithAttributes(
			&BurnFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
				Value:                        splitValue2,
				Nonce:                        transferTXR.TransactionOrder.Hash(hashAlgorithm),
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
	_, burnProof1, burnTxRecord, err := tokenPrt.GetTxProof(burnTx)
	require.NoError(t, err)

	require.NoError(t, types.VerifyTxProof(burnProof1, burnTxRecord, trustBase, hashAlgorithm))

	_, burnProof2, burnTxRecord2, err := tokenPrt.GetTxProof(burnTx2)
	require.NoError(t, err)
	require.NoError(t, types.VerifyTxProof(burnProof2, burnTxRecord2, trustBase, hashAlgorithm))
	joinTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(PayloadTypeJoinFungibleToken),
		testtransaction.WithAttributes(
			&JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTxRecord, burnTxRecord2},
				Proofs:                       []*types.TxProof{burnProof1, burnProof2},
				Backlink:                     transferTXR.TransactionOrder.Hash(hashAlgorithm),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			},
		),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(joinTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(tokenPrt, joinTx), test.WaitDuration, test.WaitTick)

	_, _, joinTXR, err := tokenPrt.GetTxProof(joinTx)
	require.NoError(t, err)
	joinTXRHash := joinTXR.TransactionOrder.Hash(gocrypto.SHA256)

	u, err := states[0].GetUnit(fungibleTokenID1, true)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data())
	d := u.Data().(*fungibleTokenData)
	require.NotNil(t, totalValue, d.value)

	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   joinTXRHash,
		bearer:     script.PredicateAlwaysTrue(),
		tokenValue: totalValue,
	})

	unit, err := state0.GetUnit(feeCreditID, true)
	require.NoError(t, err)
	require.Equal(t, uint64(92), unit.Data().(*fcunit.FeeCreditRecord).Balance)
}

func verifyProof(t *testing.T, tx *types.TransactionOrder, prt *testpartition.NodePartition, trustBase map[string]crypto.Verifier, hashAlgorithm gocrypto.Hash) {
	_, proof, record, err := prt.GetTxProof(tx)
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

func RequireFungibleTokenTypeState(t *testing.T, s *state.State, e fungibleTokenTypeUnitData) {
	t.Helper()
	u, err := s.GetUnit(e.unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, e.bearer, []byte(u.Bearer()))
	require.IsType(t, &fungibleTokenTypeData{}, u.Data())
	d := u.Data().(*fungibleTokenTypeData)
	require.Equal(t, e.tokenCreationPredicate, d.tokenCreationPredicate)
	require.Equal(t, e.subTypeCreationPredicate, d.subTypeCreationPredicate)
	require.Equal(t, e.invariantPredicate, d.invariantPredicate)
	require.Equal(t, e.symbol, d.symbol)
	require.Equal(t, e.name, d.name)
	require.Equal(t, e.icon.Type, d.icon.Type)
	require.Equal(t, e.icon.Data, d.icon.Data)
	require.Equal(t, types.UnitID(e.parentID), d.parentTypeId)
	require.Equal(t, e.decimalPlaces, d.decimalPlaces)
}

func RequireFungibleTokenState(t *testing.T, s *state.State, e fungibleTokenUnitData) {
	t.Helper()
	u, err := s.GetUnit(e.unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, e.bearer, []byte(u.Bearer()))
	require.IsType(t, &fungibleTokenData{}, u.Data())
	d := u.Data().(*fungibleTokenData)
	require.Equal(t, e.tokenValue, d.value)
	require.Equal(t, e.backlink, d.backlink)
	require.Equal(t, types.UnitID(e.typeUnitID), d.tokenType)
}

func newStateWithFeeCredit(t *testing.T, feeCreditID types.UnitID) *state.State {
	s := state.NewEmptyState()
	require.NoError(t, s.Apply(
		fcunit.AddCredit(feeCreditID, script.PredicateAlwaysTrue(), &fcunit.FeeCreditRecord{
			Balance: 100,
			Hash:    make([]byte, 32),
			Timeout: 1000,
		}),
	))
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	return s
}
