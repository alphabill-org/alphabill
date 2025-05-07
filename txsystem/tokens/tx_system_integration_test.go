package tokens

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

var (
	feeCreditID types.UnitID = append(make(types.UnitID, 31), 42, tokens.FeeCreditRecordUnitType)

	defaultClientMetadata = &types.ClientMetadata{
		Timeout:           20,
		MaxTransactionFee: 10,
		FeeCreditRecordID: feeCreditID,
	}
)

func TestInitPartitionAndDefineNFT_Ok(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     tokens.DefaultPartitionID,
		PartitionTypeID: tokens.PartitionTypeID,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       2000 * time.Millisecond,
	}
	genesisState := newStateWithFeeCredit(t, feeCreditID)
	abNet := testpartition.NewAlphabillNetwork(t, 1)
	require.NoError(t, abNet.Start(t))
	defer abNet.WaitClose(t)
	abNet.AddShard(t, &pdr, 3, func(orchestration testpartition.Orchestration) txsystem.TransactionSystem {
		system, err := NewTxSystem(pdr, orchestration, observability.Default(t), WithState(genesisState.Clone()))
		require.NoError(t, err)
		return system
	})
	tokenPrt, err := abNet.GetShard(types.PartitionShardID{PartitionID: pdr.PartitionID, ShardID: pdr.ShardID.Key()})
	require.NoError(t, err)

	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineNFT),
		testtransaction.WithPartitionID(pdr.PartitionID),
		testtransaction.WithAuthProof(&tokens.DefineNonFungibleTokenAuthProof{}),
		testtransaction.WithAttributes(&tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   "Test",
			Name:                     "Long name for Test",
			Icon:                     &tokens.Icon{Type: validIconType, Data: []byte{3, 2, 1}},
			ParentTypeID:             nil,
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			TokenMintingPredicate:    templates.AlwaysTrueBytes(),
			TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
			DataUpdatePredicate:      templates.AlwaysTrueBytes(),
		}),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	tx.UnitID, err = pdr.ComposeUnitID(types.ShardID{}, tokens.NonFungibleTokenTypeUnitType, tokens.PrndSh(tx))
	require.NoError(t, err)
	require.NoError(t, tokenPrt.BroadcastTx(tx))
	require.Eventually(t, testpartition.BlockchainContainsSuccessfulTx(t, tokenPrt, tx), test.WaitDuration, test.WaitTick)
}

func TestFungibleTokenTransactions_Ok(t *testing.T) {
	var (
		states              []*state.State
		fungibleTokenTypeID        = tokenid.NewFungibleTokenTypeID(t)
		totalValue          uint64 = 1000
		splitValue1         uint64 = 100
		splitValue2         uint64 = 10
		orchestration       Orchestration
	)
	pdr := types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     tokens.DefaultPartitionID,
		PartitionTypeID: tokens.PartitionTypeID,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       2000 * time.Millisecond,
	}

	// setup network
	genesisState := newStateWithFeeCredit(t, feeCreditID)
	// the tx system lambda is called once for node genesis, but this is not interesting so clear the states before node
	// is started
	states = []*state.State{}
	abNet := testpartition.NewAlphabillNetwork(t, 1)
	require.NoError(t, abNet.Start(t))
	defer abNet.WaitClose(t)
	abNet.AddShard(t, &pdr, 3, func(orch testpartition.Orchestration) txsystem.TransactionSystem {
		orchestration = orch
		genesisState = genesisState.Clone()
		system, err := NewTxSystem(pdr, orch, observability.Default(t), WithState(genesisState))
		require.NoError(t, err)
		states = append(states, genesisState)
		return system
	})
	tokenPrt, err := abNet.GetShard(types.PartitionShardID{PartitionID: pdr.PartitionID, ShardID: pdr.ShardID.Key()})
	require.NoError(t, err)

	state0 := states[0]

	// create fungible token type
	createTypeTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithUnitID(fungibleTokenTypeID),
		testtransaction.WithTransactionType(tokens.TransactionTypeDefineFT),
		testtransaction.WithAttributes(
			&tokens.DefineFungibleTokenAttributes{
				Symbol:                   "ALPHA",
				Name:                     "Long name for ALPHA",
				Icon:                     &tokens.Icon{Type: validIconType, Data: []byte{1, 2, 3}},
				ParentTypeID:             nil,
				SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
				TokenMintingPredicate:    templates.AlwaysTrueBytes(),
				TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
			},
		),
		testtransaction.WithAuthProof(&tokens.DefineFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(createTypeTx))
	txProof, err := testpartition.WaitTxProof(t, tokenPrt, createTypeTx)
	require.NoError(t, err, "token create type tx failed")
	require.NoError(t, txProof.Verify(orchestration.TrustBase))
	RequireFungibleTokenTypeState(t, state0, fungibleTokenTypeUnitData{
		tokenMintingPredicate:    templates.AlwaysTrueBytes(),
		subTypeCreationPredicate: templates.AlwaysTrueBytes(),
		tokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
		unitID:                   fungibleTokenTypeID,
		symbol:                   "ALPHA",
		name:                     "Long name for ALPHA",
		icon:                     &tokens.Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		parentID:                 nil,
		decimalPlaces:            0,
	})

	// mint token
	mintTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithTransactionType(tokens.TransactionTypeMintFT),
		testtransaction.WithAttributes(
			&tokens.MintFungibleTokenAttributes{
				OwnerPredicate: templates.AlwaysTrueBytes(),
				TypeID:         fungibleTokenTypeID,
				Value:          totalValue,
			},
		),
		testtransaction.WithAuthProof(tokens.MintFungibleTokenAuthProof{}),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokens.GenerateUnitID(mintTx, &pdr))
	mintedTokenID := mintTx.UnitID
	require.NoError(t, tokenPrt.BroadcastTx(mintTx))
	minTxProof, err := testpartition.WaitTxProof(t, tokenPrt, mintTx)
	require.NoError(t, err, "token mint transaction failed")
	require.NoError(t, minTxProof.Verify(orchestration.TrustBase))
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     mintedTokenID,
		typeUnitID: fungibleTokenTypeID,
		counter:    0,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue,
	})

	// split token
	splitTx1 := testtransaction.NewTransactionOrder(t,
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithUnitID(mintedTokenID),
		testtransaction.WithTransactionType(tokens.TransactionTypeSplitFT),
		testtransaction.WithAttributes(
			&tokens.SplitFungibleTokenAttributes{
				TypeID:            fungibleTokenTypeID,
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       splitValue1,
				Counter:           0,
			},
		),
		testtransaction.WithAuthProof(&tokens.SplitFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{nil}}),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(splitTx1))
	split1TxProof, err := testpartition.WaitTxProof(t, tokenPrt, splitTx1)
	require.NoError(t, err, "token split transaction failed")
	require.NoError(t, split1TxProof.Verify(orchestration.TrustBase))
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     mintedTokenID,
		typeUnitID: fungibleTokenTypeID,
		counter:    1,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue - splitValue1,
	})

	sUnitID1, err := pdr.ComposeUnitID(types.ShardID{}, tokens.FungibleTokenUnitType, tokens.PrndSh(splitTx1))
	require.NoError(t, err)
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     sUnitID1,
		typeUnitID: fungibleTokenTypeID,
		counter:    0,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: splitValue1,
	})

	splitTx2 := testtransaction.NewTransactionOrder(t,
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithUnitID(mintedTokenID),
		testtransaction.WithAuthProof(&tokens.SplitFungibleTokenAuthProof{}),
		testtransaction.WithTransactionType(tokens.TransactionTypeSplitFT),
		testtransaction.WithAttributes(
			&tokens.SplitFungibleTokenAttributes{
				TypeID:            fungibleTokenTypeID,
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       splitValue2,
				Counter:           1,
			},
		),
		testtransaction.WithAuthProof(&tokens.SplitFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{nil}}),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(splitTx2))
	split2TxProof, err := testpartition.WaitTxProof(t, tokenPrt, splitTx2)
	require.NoError(t, err, "token split 2 transaction failed")
	require.NoError(t, split2TxProof.Verify(orchestration.TrustBase))
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     mintedTokenID,
		typeUnitID: fungibleTokenTypeID,
		counter:    2,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	sUnitID2, err := pdr.ComposeUnitID(types.ShardID{}, tokens.FungibleTokenUnitType, tokens.PrndSh(splitTx2))
	require.NoError(t, err)
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     sUnitID2,
		typeUnitID: fungibleTokenTypeID,
		counter:    0,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: splitValue2,
	})

	// Transfer token
	transferTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithUnitID(mintedTokenID),
		testtransaction.WithTransactionType(tokens.TransactionTypeTransferFT),
		testtransaction.WithAttributes(
			&tokens.TransferFungibleTokenAttributes{
				TypeID:            fungibleTokenTypeID,
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				Value:             totalValue - splitValue1 - splitValue2,
				Counter:           2,
			},
		),
		testtransaction.WithAuthProof(&tokens.TransferFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{nil}}),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(transferTx))
	transferTxProof, err := testpartition.WaitTxProof(t, tokenPrt, transferTx)
	require.NoError(t, err, "token transfer transaction failed")
	require.NoError(t, transferTxProof.Verify(orchestration.TrustBase))
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     mintedTokenID,
		typeUnitID: fungibleTokenTypeID,
		counter:    3,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	// burn token x 2
	burnTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(sUnitID1),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithTransactionType(tokens.TransactionTypeBurnFT),
		testtransaction.WithAttributes(
			&tokens.BurnFungibleTokenAttributes{
				TypeID:             fungibleTokenTypeID,
				Value:              splitValue1,
				TargetTokenID:      mintedTokenID,
				TargetTokenCounter: 3,
				Counter:            0,
			},
		),
		testtransaction.WithAuthProof(&tokens.BurnFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{nil}}),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(burnTx))
	burnTxProof, err := testpartition.WaitTxProof(t, tokenPrt, burnTx)
	require.NoError(t, err, "token burn transaction failed")
	require.NoError(t, burnTxProof.Verify(orchestration.TrustBase))

	burnTx2 := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(sUnitID2),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithTransactionType(tokens.TransactionTypeBurnFT),
		testtransaction.WithAttributes(
			&tokens.BurnFungibleTokenAttributes{
				TypeID:             fungibleTokenTypeID,
				Value:              splitValue2,
				TargetTokenID:      mintedTokenID,
				TargetTokenCounter: 3,
				Counter:            0,
			},
		),
		testtransaction.WithAuthProof(&tokens.BurnFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{nil}}),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(burnTx2))
	burn2TxProof, err := testpartition.WaitTxProof(t, tokenPrt, burnTx2)
	require.NoError(t, err, "token burn 2 transaction failed")
	require.NoError(t, burn2TxProof.Verify(orchestration.TrustBase))

	txProofs := []*types.TxRecordProof{burnTxProof, burn2TxProof}
	sort.Slice(txProofs, func(i, j int) bool {
		return testtransaction.FetchTxoV1(t, txProofs[i]).UnitID.Compare(testtransaction.FetchTxoV1(t, txProofs[j]).UnitID) < 0
	})
	//var burnTxs []*types.TransactionRecord
	//var burnTxProofs []*types.TxProof
	//for _, txWithProof := range txsWithProofs {
	//	burnTxs = append(burnTxs, txWithProof.burnTx)
	//	burnTxProofs = append(burnTxProofs, txWithProof.burnTxProof)
	//}

	// join token
	joinTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithUnitID(mintedTokenID),
		testtransaction.WithTransactionType(tokens.TransactionTypeJoinFT),
		testtransaction.WithAttributes(&tokens.JoinFungibleTokenAttributes{BurnTokenProofs: txProofs}),
		testtransaction.WithAuthProof(&tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{nil}}),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(joinTx))
	joinTxProof, err := testpartition.WaitTxProof(t, tokenPrt, joinTx)
	require.NoError(t, err, "token join transaction failed")
	require.NoError(t, joinTxProof.Verify(orchestration.TrustBase))

	u, err := states[0].GetUnit(mintedTokenID, true)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)
	require.NotNil(t, totalValue, d.Value)

	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     mintedTokenID,
		typeUnitID: fungibleTokenTypeID,
		counter:    4,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue,
	})

	u, err = state0.GetUnit(feeCreditID, true)
	require.NoError(t, err)
	require.Equal(t, uint64(92), u.Data().(*fc.FeeCreditRecord).Balance)
}

type fungibleTokenUnitData struct {
	unitID     []byte
	typeUnitID []byte
	counter    uint64
	bearer     []byte
	tokenValue uint64
}

type fungibleTokenTypeUnitData struct {
	parentID, unitID                                                         []byte
	symbol, name                                                             string
	icon                                                                     *tokens.Icon
	decimalPlaces                                                            uint32
	tokenMintingPredicate, subTypeCreationPredicate, tokenTypeOwnerPredicate []byte
}

func RequireFungibleTokenTypeState(t *testing.T, s *state.State, e fungibleTokenTypeUnitData) {
	t.Helper()
	u, err := s.GetUnit(e.unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenTypeData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenTypeData)
	require.Nil(t, d.Owner())
	require.EqualValues(t, e.tokenMintingPredicate, d.TokenMintingPredicate)
	require.EqualValues(t, e.subTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.EqualValues(t, e.tokenTypeOwnerPredicate, d.TokenTypeOwnerPredicate)
	require.Equal(t, e.symbol, d.Symbol)
	require.Equal(t, e.name, d.Name)
	require.Equal(t, e.icon.Type, d.Icon.Type)
	require.Equal(t, e.icon.Data, d.Icon.Data)
	require.Equal(t, types.UnitID(e.parentID), d.ParentTypeID)
	require.Equal(t, e.decimalPlaces, d.DecimalPlaces)
}

func RequireFungibleTokenState(t *testing.T, s *state.State, e fungibleTokenUnitData) {
	t.Helper()
	u, err := s.GetUnit(e.unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)
	require.Equal(t, e.bearer, d.Owner())
	require.Equal(t, e.tokenValue, d.Value)
	require.Equal(t, e.counter, d.Counter)
	require.Equal(t, types.UnitID(e.typeUnitID), d.TypeID)
}

func newStateWithFeeCredit(t *testing.T, feeCreditID types.UnitID) *state.State {
	s := state.NewEmptyState()
	require.NoError(t, s.Apply(
		unit.AddCredit(feeCreditID, &fc.FeeCreditRecord{
			Balance:        100,
			OwnerPredicate: templates.AlwaysTrueBytes(),
			Counter:        10,
			MinLifetime:    1000,
		}),
	))
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	return s
}
