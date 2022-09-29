package tokens

import (
	gocrypto "crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const (
	invalidSymbolName = "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥"
	validSymbolName   = "BETA"
	validUnitID       = 10
	existingUnitID    = 1
)

func TestCreateFungibleTokenType_NotOk(t *testing.T) {
	executor := &createFungibleTokenTypeTxExecutor{
		baseTxExecutor: &baseTxExecutor[*fungibleTokenTypeData]{
			state:         initState(t),
			hashAlgorithm: gocrypto.SHA256,
		},
	}

	tests := []struct {
		name       string
		tx         txsystem.GenericTransaction
		wantErrStr string
	}{
		{
			name:       "invalid tx type",
			tx:         &createNonFungibleTokenTypeWrapper{},
			wantErrStr: fmt.Sprintf("invalid tx type: %T", &createNonFungibleTokenTypeWrapper{}),
		},
		{
			name:       "unit ID is 0",
			tx:         createTx(t, uint256.NewInt(0), &CreateFungibleTokenTypeAttributes{}),
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name:       "symbol name exceeds the allowed maximum length",
			tx:         createTx(t, uint256.NewInt(validUnitID), &CreateFungibleTokenTypeAttributes{Symbol: invalidSymbolName}),
			wantErrStr: "symbol name exceeds the allowed maximum length of 64 bytes",
		},
		{
			name:       "decimal places > 8",
			tx:         createTx(t, uint256.NewInt(validUnitID), &CreateFungibleTokenTypeAttributes{Symbol: validSymbolName, DecimalPlaces: 9}),
			wantErrStr: "invalid decimal places. maximum allowed value 8, got 9",
		},
		{
			name:       "unit with given ID exists",
			tx:         createTx(t, uint256.NewInt(existingUnitID), &CreateFungibleTokenTypeAttributes{Symbol: validSymbolName, DecimalPlaces: 5}),
			wantErrStr: fmt.Sprintf("unit %v exists", existingUnitID),
		},
		{
			name:       "parent.decimals != tx.attributes.decimalPlaces",
			tx:         createTx(t, uint256.NewInt(validUnitID), &CreateFungibleTokenTypeAttributes{Symbol: validSymbolName, DecimalPlaces: 6, ParentTypeId: uint256.NewInt(existingUnitID).Bytes()}),
			wantErrStr: "invalid decimal places. allowed 5, got 6",
		},
		{
			name:       "parent does not exist",
			tx:         createTx(t, uint256.NewInt(validUnitID), &CreateFungibleTokenTypeAttributes{Symbol: validSymbolName, DecimalPlaces: 6, ParentTypeId: uint256.NewInt(100).Bytes()}),
			wantErrStr: "item 100 does not exist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, executor.Execute(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestCreateFungibleTokenType_CreateSingleType_Ok(t *testing.T) {
	executor := &createFungibleTokenTypeTxExecutor{
		baseTxExecutor: &baseTxExecutor[*fungibleTokenTypeData]{
			state:         initState(t),
			hashAlgorithm: gocrypto.SHA256,
		},
	}
	attributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbolName,
		ParentTypeId:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: script.PredicateAlwaysFalse(),
		TokenCreationPredicate:   script.PredicateAlwaysTrue(),
		InvariantPredicate:       script.PredicatePayToPublicKeyHashDefault(make([]byte, 32)),
	}

	uID := uint256.NewInt(validUnitID)
	err := executor.Execute(createTx(t, uID, attributes), 10)
	require.NoError(t, err)

	u, err := executor.state.GetUnit(uID)
	require.NoError(t, err)
	require.NotNil(t, u)

	require.IsType(t, &fungibleTokenTypeData{}, u.Data)
	d := u.Data.(*fungibleTokenTypeData)
	require.Equal(t, attributes.Symbol, d.symbol)
	require.Equal(t, attributes.DecimalPlaces, d.decimalPlaces)
	require.Equal(t, attributes.SubTypeCreationPredicate, d.subTypeCreationPredicate)
	require.Equal(t, attributes.TokenCreationPredicate, d.tokenCreationPredicate)
	require.Equal(t, attributes.InvariantPredicate, d.invariantPredicate)
	require.Equal(t, uint256.NewInt(0), d.parentTypeId)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_Ok(t *testing.T) {
	executor := &createFungibleTokenTypeTxExecutor{
		baseTxExecutor: &baseTxExecutor[*fungibleTokenTypeData]{
			state:         initState(t),
			hashAlgorithm: gocrypto.SHA256,
		},
	}

	parentAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbolName,
		ParentTypeId:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
		TokenCreationPredicate:   script.PredicateAlwaysFalse(),
		InvariantPredicate:       script.PredicatePayToPublicKeyHashDefault(make([]byte, 32)),
	}
	parentID := uint256.NewInt(validUnitID)
	childID := uint256.NewInt(20)
	childAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                            validSymbolName + "_CHILD",
		ParentTypeId:                      parentID.Bytes(),
		DecimalPlaces:                     6,
		SubTypeCreationPredicate:          script.PredicateAlwaysFalse(),
		TokenCreationPredicate:            script.PredicateAlwaysTrue(),
		InvariantPredicate:                script.PredicateAlwaysTrue(),
		SubTypeCreationPredicateSignature: script.PredicateArgumentEmpty(),
	}

	err := executor.Execute(createTx(t, parentID, parentAttributes), 10)
	require.NoError(t, err)

	err = executor.Execute(createTx(t, childID, childAttributes), 11)
	require.NoError(t, err)

	u, err := executor.state.GetUnit(childID)
	require.NoError(t, err)
	require.NotNil(t, u)

	require.IsType(t, &fungibleTokenTypeData{}, u.Data)
	d := u.Data.(*fungibleTokenTypeData)
	require.Equal(t, childAttributes.Symbol, d.symbol)
	require.Equal(t, childAttributes.DecimalPlaces, d.decimalPlaces)
	require.Equal(t, childAttributes.SubTypeCreationPredicate, d.subTypeCreationPredicate)
	require.Equal(t, childAttributes.TokenCreationPredicate, d.tokenCreationPredicate)
	require.Equal(t, childAttributes.InvariantPredicate, d.invariantPredicate)
	require.Equal(t, parentID, d.parentTypeId)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_InvalidCreationPredicateSiganture(t *testing.T) {
	executor := &createFungibleTokenTypeTxExecutor{
		baseTxExecutor: &baseTxExecutor[*fungibleTokenTypeData]{
			state:         initState(t),
			hashAlgorithm: gocrypto.SHA256,
		},
	}

	parentAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbolName,
		ParentTypeId:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
		TokenCreationPredicate:   script.PredicateAlwaysFalse(),
		InvariantPredicate:       script.PredicatePayToPublicKeyHashDefault(make([]byte, 32)),
	}
	parentID := uint256.NewInt(validUnitID)
	childID := uint256.NewInt(20)
	childAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                            validSymbolName + "_CHILD",
		ParentTypeId:                      parentID.Bytes(),
		DecimalPlaces:                     6,
		SubTypeCreationPredicate:          script.PredicateAlwaysFalse(),
		TokenCreationPredicate:            script.PredicateAlwaysTrue(),
		InvariantPredicate:                script.PredicateAlwaysTrue(),
		SubTypeCreationPredicateSignature: []byte("invalid"),
	}

	err := executor.Execute(createTx(t, parentID, parentAttributes), 10)
	require.NoError(t, err)

	err = executor.Execute(createTx(t, childID, childAttributes), 11)
	require.ErrorContains(t, err, "invalid script format")
}

func initState(t *testing.T) *rma.Tree {
	state, err := rma.New(&rma.Config{
		HashAlgorithm: gocrypto.SHA256,
	})
	state.AddItem(uint256.NewInt(existingUnitID), script.PredicateAlwaysTrue(), &fungibleTokenTypeData{
		symbol:                   "ALPHA",
		parentTypeId:             uint256.NewInt(0),
		decimalPlaces:            5,
		subTypeCreationPredicate: script.PredicateAlwaysTrue(),
		tokenCreationPredicate:   script.PredicateAlwaysTrue(),
		invariantPredicate:       script.PredicateAlwaysTrue(),
	}, make([]byte, 32))
	require.NoError(t, err)
	return state
}

func createTx(t *testing.T, unitID *uint256.Int, attributes proto.Message) txsystem.GenericTransaction {
	return testtransaction.NewGenericTransaction(
		t,
		NewGenericTx,
		testtransaction.WithUnitId(unitID.Bytes()),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(attributes),
	)
}
