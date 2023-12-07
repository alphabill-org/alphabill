package tokens

import (
	gocrypto "crypto"
	"sort"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

var feeCreditID = NewFeeCreditRecordID(nil, []byte{42})
var defaultClientMetadata = &types.ClientMetadata{
	Timeout:           20,
	MaxTransactionFee: 10,
	FeeCreditRecordID: feeCreditID,
}

func TestInitPartitionAndCreateNFTType_Ok(t *testing.T) {
	tokenPrt, err := testpartition.NewPartition(t, 3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := NewTxSystem(logger.New(t), WithTrustBase(trustBase), WithState(newStateWithFeeCredit(t, feeCreditID)))
		require.NoError(t, err)
		return system
	}, DefaultSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{tokenPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start(t))
	defer abNet.WaitClose(t)

	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithPayloadType(PayloadTypeCreateNFTType),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(NewNonFungibleTokenTypeID(nil, []byte{1})),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithAttributes(
			&CreateNonFungibleTokenTypeAttributes{
				Symbol:                   "Test",
				Name:                     "Long name for Test",
				Icon:                     &Icon{Type: validIconType, Data: []byte{3, 2, 1}},
				ParentTypeID:             nil,
				SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
				TokenCreationPredicate:   templates.AlwaysTrueBytes(),
				InvariantPredicate:       templates.AlwaysTrueBytes(),
				DataUpdatePredicate:      templates.AlwaysTrueBytes(),
			},
		),
		testtransaction.WithFeeProof(nil),
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
	tokenPrt, err := testpartition.NewPartition(t, 1, func(tb map[string]crypto.Verifier) txsystem.TransactionSystem {
		trustBase = tb
		s := newStateWithFeeCredit(t, feeCreditID)
		system, err := NewTxSystem(logger.New(t), WithState(s), WithTrustBase(tb))
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
	require.NoError(t, abNet.Start(t))
	defer abNet.WaitClose(t)

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
				SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
				TokenCreationPredicate:   templates.AlwaysTrueBytes(),
				InvariantPredicate:       templates.AlwaysTrueBytes(),
			},
		),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(createTypeTx))
	txRecord, txProof, err := testpartition.WaitTxProof(t, tokenPrt, createTypeTx)
	require.NoError(t, err, "token create type tx failed")
	RequireFungibleTokenTypeState(t, state0, fungibleTokenTypeUnitData{
		tokenCreationPredicate:   templates.AlwaysTrueBytes(),
		subTypeCreationPredicate: templates.AlwaysTrueBytes(),
		invariantPredicate:       templates.AlwaysTrueBytes(),
		unitID:                   fungibleTokenTypeID,
		bearer:                   templates.AlwaysTrueBytes(),
		symbol:                   "ALPHA",
		name:                     "Long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		parentID:                 nil,
		decimalPlaces:            0,
	})
	require.NoError(t, types.VerifyTxProof(txProof, txRecord, trustBase, hashAlgorithm))

	// mint token
	mintTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithPayloadType(PayloadTypeMintFungibleToken),
		testtransaction.WithAttributes(
			&MintFungibleTokenAttributes{
				Bearer:                           templates.AlwaysTrueBytes(),
				TypeID:                           fungibleTokenTypeID,
				Value:                            totalValue,
				TokenCreationPredicateSignatures: [][]byte{nil},
			},
		),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(mintTx))
	mintTxRecord, minTxProof, err := testpartition.WaitTxProof(t, tokenPrt, mintTx)
	require.NoError(t, err, "token mint tx failed")
	txHash := mintTxRecord.TransactionOrder.Hash(gocrypto.SHA256)

	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   txHash,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue,
	})
	require.NoError(t, types.VerifyTxProof(minTxProof, mintTxRecord, trustBase, hashAlgorithm))

	// split token
	splitTx1 := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(PayloadTypeSplitFungibleToken),
		testtransaction.WithAttributes(
			&SplitFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  splitValue1,
				RemainingValue:               totalValue - splitValue1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     txHash,
				InvariantPredicateSignatures: [][]byte{nil},
			},
		),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(splitTx1))
	split1TxRecord, split1TxProof, err := testpartition.WaitTxProof(t, tokenPrt, splitTx1)
	require.NoError(t, err, "token split tx failed")
	split1GenTxHash := split1TxRecord.TransactionOrder.Hash(gocrypto.SHA256)

	require.NoError(t, err)
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   split1GenTxHash,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue - splitValue1,
	})
	require.NoError(t, types.VerifyTxProof(split1TxProof, split1TxRecord, trustBase, hashAlgorithm))

	sUnitID1 := NewFungibleTokenID(fungibleTokenID1, HashForIDCalculation(splitTx1, hashAlgorithm))
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     sUnitID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   split1GenTxHash,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: splitValue1,
	})

	splitTx2 := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(PayloadTypeSplitFungibleToken),
		testtransaction.WithAttributes(
			&SplitFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  splitValue2,
				RemainingValue:               totalValue - (splitValue1 + splitValue2),
				Nonce:                        nil,
				Backlink:                     split1TxRecord.TransactionOrder.Hash(hashAlgorithm),
				InvariantPredicateSignatures: [][]byte{nil},
			},
		),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(splitTx2))
	split2TxRecord, split2TxProof, err := testpartition.WaitTxProof(t, tokenPrt, splitTx2)
	require.NoError(t, err, "token split 2 tx failed")
	require.NoError(t, types.VerifyTxProof(split2TxProof, split2TxRecord, trustBase, hashAlgorithm))

	splitGenTx2Hash := split2TxRecord.TransactionOrder.Hash(gocrypto.SHA256)
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   splitGenTx2Hash,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	sUnitID2 := NewFungibleTokenID(fungibleTokenID1, HashForIDCalculation(splitTx2, hashAlgorithm))
	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     sUnitID2,
		typeUnitID: fungibleTokenTypeID,
		backlink:   splitGenTx2Hash,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: splitValue2,
	})

	// Transfer token
	transferTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(PayloadTypeTransferFungibleToken),
		testtransaction.WithAttributes(
			&TransferFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        totalValue - splitValue1 - splitValue2,
				Nonce:                        nil,
				Backlink:                     splitGenTx2Hash,
				InvariantPredicateSignatures: [][]byte{nil},
			},
		),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(transferTx))
	transferTxRecord, transferTxProof, err := testpartition.WaitTxProof(t, tokenPrt, transferTx)
	require.NoError(t, err, "token transfer tx failed")
	require.NoError(t, types.VerifyTxProof(transferTxProof, transferTxRecord, trustBase, hashAlgorithm))

	transferGenTxHash := transferTxRecord.TransactionOrder.Hash(gocrypto.SHA256)

	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   transferGenTxHash,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue - splitValue1 - splitValue2,
	})

	// burn token x 2
	burnTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(sUnitID1),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(PayloadTypeBurnFungibleToken),
		testtransaction.WithAttributes(
			&BurnFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
				Value:                        splitValue1,
				TargetTokenID:                fungibleTokenID1,
				TargetTokenBacklink:          transferGenTxHash,
				Backlink:                     split1GenTxHash,
				InvariantPredicateSignatures: [][]byte{nil},
			},
		),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(burnTx))
	burnTxRecord, burnTxProof, err := testpartition.WaitTxProof(t, tokenPrt, burnTx)
	require.NoError(t, err, "token burn tx failed")
	require.NoError(t, types.VerifyTxProof(burnTxProof, burnTxRecord, trustBase, hashAlgorithm))

	burnTx2 := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(sUnitID2),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(PayloadTypeBurnFungibleToken),
		testtransaction.WithAttributes(
			&BurnFungibleTokenAttributes{
				TypeID:                       fungibleTokenTypeID,
				Value:                        splitValue2,
				TargetTokenID:                fungibleTokenID1,
				TargetTokenBacklink:          transferTxRecord.TransactionOrder.Hash(hashAlgorithm),
				Backlink:                     splitGenTx2Hash,
				InvariantPredicateSignatures: [][]byte{nil},
			},
		),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(burnTx2))
	burn2TxRecord, burn2TxProof, err := testpartition.WaitTxProof(t, tokenPrt, burnTx2)
	require.NoError(t, err, "token burn 2 tx failed")
	require.NoError(t, types.VerifyTxProof(burn2TxProof, burn2TxRecord, trustBase, hashAlgorithm))

	// group txs with proofs, and sort by unit id
	type txWithProof struct {
		burnTx      *types.TransactionRecord
		burnTxProof *types.TxProof
	}
	txsWithProofs := []*txWithProof{
		{burnTx: burnTxRecord, burnTxProof: burnTxProof},
		{burnTx: burn2TxRecord, burnTxProof: burn2TxProof},
	}
	sort.Slice(txsWithProofs, func(i, j int) bool {
		return txsWithProofs[i].burnTx.TransactionOrder.UnitID().Compare(txsWithProofs[j].burnTx.TransactionOrder.UnitID()) < 0
	})
	var burnTxs []*types.TransactionRecord
	var burnTxProofs []*types.TxProof
	for _, txWithProof := range txsWithProofs {
		burnTxs = append(burnTxs, txWithProof.burnTx)
		burnTxProofs = append(burnTxProofs, txWithProof.burnTxProof)
	}

	// join token
	joinTx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithUnitId(fungibleTokenID1),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(PayloadTypeJoinFungibleToken),
		testtransaction.WithAttributes(
			&JoinFungibleTokenAttributes{
				BurnTransactions:             burnTxs,
				Proofs:                       burnTxProofs,
				Backlink:                     transferTxRecord.TransactionOrder.Hash(hashAlgorithm),
				InvariantPredicateSignatures: [][]byte{nil},
			},
		),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(createClientMetadata()),
	)
	require.NoError(t, tokenPrt.BroadcastTx(joinTx))
	joinTxRecord, joinTxProof, err := testpartition.WaitTxProof(t, tokenPrt, joinTx)
	require.NoError(t, err, "token join tx failed")
	require.NoError(t, types.VerifyTxProof(joinTxProof, joinTxRecord, trustBase, hashAlgorithm))
	joinTXRHash := joinTxRecord.TransactionOrder.Hash(gocrypto.SHA256)

	u, err := states[0].GetUnit(fungibleTokenID1, true)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &FungibleTokenData{}, u.Data())
	d := u.Data().(*FungibleTokenData)
	require.NotNil(t, totalValue, d.Value)

	RequireFungibleTokenState(t, state0, fungibleTokenUnitData{
		unitID:     fungibleTokenID1,
		typeUnitID: fungibleTokenTypeID,
		backlink:   joinTXRHash,
		bearer:     templates.AlwaysTrueBytes(),
		tokenValue: totalValue,
	})

	u, err = state0.GetUnit(feeCreditID, true)
	require.NoError(t, err)
	require.Equal(t, uint64(92), u.Data().(*unit.FeeCreditRecord).Balance)
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
	require.IsType(t, &FungibleTokenTypeData{}, u.Data())
	d := u.Data().(*FungibleTokenTypeData)
	require.Equal(t, e.tokenCreationPredicate, d.TokenCreationPredicate)
	require.Equal(t, e.subTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.Equal(t, e.invariantPredicate, d.InvariantPredicate)
	require.Equal(t, e.symbol, d.Symbol)
	require.Equal(t, e.name, d.Name)
	require.Equal(t, e.icon.Type, d.Icon.Type)
	require.Equal(t, e.icon.Data, d.Icon.Data)
	require.Equal(t, types.UnitID(e.parentID), d.ParentTypeId)
	require.Equal(t, e.decimalPlaces, d.DecimalPlaces)
}

func RequireFungibleTokenState(t *testing.T, s *state.State, e fungibleTokenUnitData) {
	t.Helper()
	u, err := s.GetUnit(e.unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, e.bearer, []byte(u.Bearer()))
	require.IsType(t, &FungibleTokenData{}, u.Data())
	d := u.Data().(*FungibleTokenData)
	require.Equal(t, e.tokenValue, d.Value)
	require.Equal(t, e.backlink, d.backlink)
	require.Equal(t, types.UnitID(e.typeUnitID), d.TokenType)
}

func newStateWithFeeCredit(t *testing.T, feeCreditID types.UnitID) *state.State {
	s := state.NewEmptyState()
	require.NoError(t, s.Apply(
		unit.AddCredit(feeCreditID, templates.AlwaysTrueBytes(), &unit.FeeCreditRecord{
			Balance:  100,
			Backlink: make([]byte, 32),
			Timeout:  1000,
		}),
	))
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	return s
}
