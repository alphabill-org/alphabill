package tokens

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

const (
	invalidSymbol   = "♥ Alphabill ♥"
	invalidName     = "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill♥♥"
	invalidIconType = "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥"
	validSymbol     = "BETA"
	validName       = "Long name for BETA"
	validIconType   = "image/png"

	existingTokenValue = 1000
)

var (
	existingTokenTypeID  types.UnitID = append(make([]byte, 31), 1, tokens.FungibleTokenTypeUnitType)
	existingTokenTypeID2 types.UnitID = append(make([]byte, 31), 11, tokens.FungibleTokenTypeUnitType)

	existingTokenID  types.UnitID = append(make([]byte, 31), 0x02, tokens.FungibleTokenUnitType)
	existingTokenID2 types.UnitID = append(make([]byte, 31), 0xaa, tokens.FungibleTokenUnitType)
	existingTokenID3 types.UnitID = append(make([]byte, 31), 0xbb, tokens.FungibleTokenUnitType)

	nonExistingTokenTypeID types.UnitID = append(make([]byte, 31), 100, tokens.FungibleTokenTypeUnitType)
	nonExistingTokenID     types.UnitID = append(make([]byte, 31), 100, tokens.FungibleTokenUnitType)

	invalidFungibleTokenTypeID types.UnitID = append(make([]byte, 31), 2, tokens.NonFungibleTokenTypeUnitType) // use non-fungible type id
	invalidFungibleTokenID     types.UnitID = append(make([]byte, 31), 1, tokens.NonFungibleTokenUnitType)     // use non-fungible type id
)

func TestCreateFungibleTokenType_NotOk(t *testing.T) {
	ftTypeID := tokenid.NewFungibleTokenTypeID(t)
	ftTokenID := tokenid.NewFungibleTokenID(t)

	validTxOrder := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			Type:   tokens.TransactionTypeDefineFT,
			UnitID: ftTypeID,
		},
	}
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.DefineFungibleTokenAttributes
		authProof  *tokens.DefineFungibleTokenAuthProof
		options    *Options
		wantErrStr string
	}{
		{
			name: "nil unitID",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeDefineFT,
					UnitID: nil,
				},
			},
			attr:       &tokens.DefineFungibleTokenAttributes{},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "invalid unit ID",
		},
		{
			name: "unitID has wrong type",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeDefineFT,
					UnitID: existingTokenID,
				},
			},
			attr:       &tokens.DefineFungibleTokenAttributes{},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "invalid unit ID",
		},
		{
			name: "parentTypeID has wrong type",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeDefineFT,
					UnitID: existingTokenTypeID,
				},
			},
			attr:       &tokens.DefineFungibleTokenAttributes{ParentTypeID: ftTokenID},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "invalid parent type: expected type 0X1, got 0X3",
		},
		{
			name:       "symbol length exceeds the allowed maximum",
			tx:         validTxOrder,
			attr:       &tokens.DefineFungibleTokenAttributes{Symbol: invalidSymbol},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "symbol length exceeds the allowed maximum of 16 bytes",
		},
		{
			name:       "name length exceeds the allowed maximum",
			tx:         validTxOrder,
			attr:       &tokens.DefineFungibleTokenAttributes{Name: invalidName},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "name length exceeds the allowed maximum of 256 bytes",
		},
		{
			name: "icon type length exceeds the allowed maximum",
			tx:   validTxOrder,
			attr: &tokens.DefineFungibleTokenAttributes{
				Symbol: validSymbol,
				Icon:   &tokens.Icon{Type: invalidIconType, Data: test.RandomBytes(maxIconDataLength)},
			},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "icon type length exceeds the allowed maximum of 64 bytes",
		},
		{
			name: "icon data length exceeds the allowed maximum",

			tx: validTxOrder,
			attr: &tokens.DefineFungibleTokenAttributes{
				Symbol: validSymbol,
				Icon:   &tokens.Icon{Type: validIconType, Data: test.RandomBytes(maxIconDataLength + 1)},
			},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "icon data length exceeds the allowed maximum of 64 KiB",
		},
		{
			name:       "decimal places > 8",
			tx:         validTxOrder,
			attr:       &tokens.DefineFungibleTokenAttributes{Symbol: validSymbol, DecimalPlaces: 9},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "invalid decimal places. maximum allowed value 8, got 9",
		},
		{
			name: "unit with given ID exists",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeDefineFT,
					UnitID: existingTokenTypeID,
				},
			},
			attr:       &tokens.DefineFungibleTokenAttributes{Symbol: validSymbol, DecimalPlaces: 5},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: fmt.Sprintf("unit %s exists", existingTokenTypeID),
		},
		{
			name:       "parent.decimals != tx.attributes.decimalPlaces",
			tx:         validTxOrder,
			attr:       &tokens.DefineFungibleTokenAttributes{Symbol: validSymbol, DecimalPlaces: 6, ParentTypeID: existingTokenTypeID},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "invalid decimal places. allowed 5, got 6",
		},
		{
			name:       "parent does not exist",
			tx:         validTxOrder,
			attr:       &tokens.DefineFungibleTokenAttributes{Symbol: validSymbol, DecimalPlaces: 6, ParentTypeID: ftTypeID},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: fmt.Sprintf("item %s does not exist", ftTypeID),
		},
	}

	m, err := NewFungibleTokensModule(tokenid.PDR(), defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = m.validateDefineFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestCreateFungibleTokenType_CreateSingleType_Ok(t *testing.T) {
	opts := defaultOpts(t)
	attributes := &tokens.DefineFungibleTokenAttributes{
		Symbol:                   validSymbol,
		Name:                     validName,
		Icon:                     &tokens.Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeID:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.AlwaysFalseBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
	}
	unitID := tokenid.NewFungibleTokenTypeID(t)
	pdr := types.PartitionDescriptionRecord{
		PartitionID:     tokens.DefaultPartitionID,
		PartitionTypeID: tokens.PartitionTypeID,
		UnitIDLen:       8 * 32,
		TypeIDLen:       8,
	}

	m, err := NewFungibleTokensModule(pdr, opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	txo := createTxOrder(t, unitID, tokens.TransactionTypeDefineFT, attributes)
	sm, err := txExecutors.ValidateAndExecute(
		txo,
		testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)),
	)
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := opts.state.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	require.IsType(t, &tokens.FungibleTokenTypeData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenTypeData)
	require.Equal(t, attributes.Symbol, d.Symbol)
	require.Equal(t, attributes.Name, d.Name)
	require.Equal(t, attributes.Icon.Type, d.Icon.Type)
	require.Equal(t, attributes.Icon.Data, d.Icon.Data)
	require.Equal(t, attributes.DecimalPlaces, d.DecimalPlaces)
	require.EqualValues(t, attributes.SubTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.EqualValues(t, attributes.TokenMintingPredicate, d.TokenMintingPredicate)
	require.EqualValues(t, attributes.TokenTypeOwnerPredicate, d.TokenTypeOwnerPredicate)
	require.Nil(t, d.ParentTypeID)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_Ok(t *testing.T) {
	opts := defaultOpts(t)
	pdr := types.PartitionDescriptionRecord{
		PartitionID:     tokens.DefaultPartitionID,
		PartitionTypeID: tokens.PartitionTypeID,
		UnitIDLen:       8 * 32,
		TypeIDLen:       8,
	}

	parentAttributes := &tokens.DefineFungibleTokenAttributes{
		Symbol:                   validSymbol,
		Name:                     validName,
		Icon:                     &tokens.Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeID:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenMintingPredicate:    templates.AlwaysFalseBytes(),
		TokenTypeOwnerPredicate:  templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
	}
	parentID := tokenid.NewFungibleTokenTypeID(t)
	parentTx := createTxOrder(t, parentID, tokens.TransactionTypeDefineFT, parentAttributes)

	childID := tokenid.NewFungibleTokenTypeID(t)
	childAttributes := &tokens.DefineFungibleTokenAttributes{
		Symbol:                   validSymbol + "_CHILD",
		Name:                     validName + "_CHILD",
		Icon:                     &tokens.Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeID:             parentID,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.AlwaysFalseBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
	}
	childTx := createTxOrder(t, childID, tokens.TransactionTypeDefineFT, childAttributes, testtransaction.WithAuthProof(tokens.DefineFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{templates.EmptyArgument()}}))

	m, err := NewFungibleTokensModule(pdr, opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	exeCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(10))
	sm, err := txExecutors.ValidateAndExecute(parentTx, exeCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	sm, err = txExecutors.ValidateAndExecute(childTx, exeCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(childID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	require.IsType(t, &tokens.FungibleTokenTypeData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenTypeData)
	require.Equal(t, childAttributes.Symbol, d.Symbol)
	require.Equal(t, childAttributes.Name, d.Name)
	require.Equal(t, childAttributes.Icon.Type, d.Icon.Type)
	require.Equal(t, childAttributes.Icon.Data, d.Icon.Data)
	require.Equal(t, childAttributes.DecimalPlaces, d.DecimalPlaces)
	require.EqualValues(t, childAttributes.SubTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.EqualValues(t, childAttributes.TokenMintingPredicate, d.TokenMintingPredicate)
	require.EqualValues(t, childAttributes.TokenTypeOwnerPredicate, d.TokenTypeOwnerPredicate)
	require.Equal(t, parentID, d.ParentTypeID)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_InvalidCreationProof(t *testing.T) {
	opts := defaultOpts(t)

	parentAttributes := &tokens.DefineFungibleTokenAttributes{
		Symbol:                   validSymbol,
		ParentTypeID:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
		TokenMintingPredicate:    templates.AlwaysFalseBytes(),
		TokenTypeOwnerPredicate:  templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
	}
	parentID := tokenid.NewFungibleTokenTypeID(t)
	parentTx := createTxOrder(t, parentID, tokens.TransactionTypeDefineFT, parentAttributes)

	childID := tokenid.NewFungibleTokenTypeID(t)
	childAttributes := &tokens.DefineFungibleTokenAttributes{
		Symbol:                   validSymbol + "_CHILD",
		ParentTypeID:             parentID,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.AlwaysFalseBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
	}
	childTx := createTxOrder(t, childID, tokens.TransactionTypeDefineFT, childAttributes, testtransaction.WithAuthProof(tokens.DefineFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{[]byte("invalid")}}))
	m, err := NewFungibleTokensModule(tokenid.PDR(), opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	sm, err := txExecutors.ValidateAndExecute(parentTx, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	sm, err = txExecutors.ValidateAndExecute(childTx, testctx.NewMockExecutionContext(testctx.WithCurrentRound(112)))
	require.EqualError(t, err, `transaction validation failed (type=1): SubTypeCreationPredicate: executing predicate [0] in the chain: executing predicate: failed to decode P2PKH256 signature: unexpected EOF`)
	require.Nil(t, sm)
}

func TestMintFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.MintFungibleTokenAttributes
		authProof  *tokens.MintFungibleTokenAuthProof
		wantErrStr string
	}{
		{
			name: "unit ID is nil",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeMintFT,
					UnitID: nil,
				},
			},
			attr:       &tokens.MintFungibleTokenAttributes{},
			authProof:  &tokens.MintFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name: "unit ID has wrong type",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeMintFT,
					UnitID: invalidFungibleTokenID,
				},
			},
			attr:       &tokens.MintFungibleTokenAttributes{TypeID: existingTokenTypeID},
			authProof:  &tokens.MintFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name: "type ID has wrong type",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeMintFT,
					UnitID: existingTokenID,
				},
			},
			attr:       &tokens.MintFungibleTokenAttributes{TypeID: invalidFungibleTokenTypeID},
			authProof:  &tokens.MintFungibleTokenAuthProof{},
			wantErrStr: "invalid token type: expected type 0X1, got 0X2",
		},
		{
			name: "unit type with given ID does not exist",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeMintFT,
					UnitID: nonExistingTokenID,
				},
			},
			attr:       &tokens.MintFungibleTokenAttributes{TypeID: nonExistingTokenTypeID},
			authProof:  &tokens.MintFungibleTokenAuthProof{},
			wantErrStr: "token type does not exist",
		},
		{
			name: "token type does not exist",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeMintFT,
					UnitID: nonExistingTokenID,
				},
			},
			attr: &tokens.MintFungibleTokenAttributes{
				OwnerPredicate: templates.AlwaysTrueBytes(),
				TypeID:         nonExistingTokenTypeID,
				Value:          1000,
			},
			authProof:  &tokens.MintFungibleTokenAuthProof{},
			wantErrStr: "token type does not exist",
		},
		{
			name: "invalid value - zero",
			tx: &types.TransactionOrder{
				Version: 1,
				Payload: types.Payload{
					Type:   tokens.TransactionTypeMintFT,
					UnitID: nonExistingTokenID,
				},
			},
			attr: &tokens.MintFungibleTokenAttributes{
				OwnerPredicate: templates.AlwaysTrueBytes(),
				TypeID:         existingTokenTypeID,
				Value:          0,
			},
			authProof:  &tokens.MintFungibleTokenAuthProof{},
			wantErrStr: `token must have value greater than zero`,
		},
	}

	m, err := NewFungibleTokensModule(tokenid.PDR(), defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateMintFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestMintFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	pdr := tokenid.PDR()
	attributes := &tokens.MintFungibleTokenAttributes{
		OwnerPredicate: templates.AlwaysTrueBytes(),
		TypeID:         existingTokenTypeID,
		Value:          existingTokenValue,
	}
	tx := createTxOrder(t, nil, tokens.TransactionTypeMintFT, attributes)
	tx.NetworkID = pdr.NetworkID
	require.NoError(t, tokens.GenerateUnitID(tx, &pdr))
	m, err := NewFungibleTokensModule(pdr, opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	sm, err := txExecutors.ValidateAndExecute(tx, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := opts.state.GetUnit(tx.UnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())

	d := u.Data().(*tokens.FungibleTokenData)
	require.Equal(t, existingTokenTypeID, d.TypeID)
	require.Equal(t, attributes.Value, d.Value)
	require.Equal(t, uint64(0), d.Counter)
	require.Equal(t, uint64(1000), d.MinLifetime)
	require.Equal(t, attributes.OwnerPredicate, d.Owner())
}

func TestTransferFungibleToken_NotOk(t *testing.T) {
	ftUnitID := tokenid.NewFungibleTokenID(t)
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.TransferFungibleTokenAttributes
		authProof  *tokens.TransferFungibleTokenAuthProof
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxOrder(t, nil, tokens.TransactionTypeTransferFT, nil),
			attr:       &tokens.TransferFungibleTokenAttributes{},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxOrder(t, existingTokenTypeID, tokens.TransactionTypeTransferFT, nil),
			attr:       &tokens.TransferFungibleTokenAttributes{},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxOrder(t, ftUnitID, tokens.TransactionTypeTransferFT, nil),
			attr:       &tokens.TransferFungibleTokenAttributes{},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", ftUnitID),
		},
		{
			name: "invalid value",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeTransferFT, &tokens.TransferFungibleTokenAttributes{}),
			attr: &tokens.TransferFungibleTokenAttributes{
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				Value:             existingTokenValue - 1,
				Counter:           0,
			},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid counter",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeTransferFT, &tokens.TransferFungibleTokenAttributes{}),
			attr: &tokens.TransferFungibleTokenAttributes{
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				Value:             existingTokenValue,
				Counter:           1,
			},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: "invalid counter",
		},
		{
			name: "empty token type id",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeTransferFT, &tokens.TransferFungibleTokenAttributes{}),
			attr: &tokens.TransferFungibleTokenAttributes{
				TypeID:            nil,
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				Value:             existingTokenValue,
				Counter:           0,
			},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token type id",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeTransferFT, &tokens.TransferFungibleTokenAttributes{}),
			attr: &tokens.TransferFungibleTokenAttributes{
				TypeID:            existingTokenTypeID2,
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				Value:             existingTokenValue,
				Counter:           0,
			},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: "invalid type identifier",
		},
	}

	m, err := NewFungibleTokensModule(tokenid.PDR(), defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateTransferFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestTransferFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	transferAttributes := &tokens.TransferFungibleTokenAttributes{
		TypeID:            existingTokenTypeID,
		NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
		Value:             existingTokenValue,
		Counter:           0,
	}
	authProof := &tokens.TransferFungibleTokenAuthProof{
		TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()},
	}
	uID := existingTokenID
	tx := createTxOrder(t, uID, tokens.TransactionTypeTransferFT, transferAttributes, testtransaction.WithAuthProof(authProof))
	m, err := NewFungibleTokensModule(tokenid.PDR(), opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	var roundNo uint64 = 10
	require.NoError(t, m.validateTransferFT(tx, transferAttributes, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo))))
	sm, err := m.executeTransferFT(tx, transferAttributes, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)

	require.Equal(t, transferAttributes.NewOwnerPredicate, d.Owner())
	require.Equal(t, transferAttributes.Value, d.Value)
	require.Equal(t, uint64(1), d.Counter)
}

func TestSplitFungibleToken_NotOk(t *testing.T) {
	ftUnitID := tokenid.NewFungibleTokenID(t)
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.SplitFungibleTokenAttributes
		authProof  *tokens.SplitFungibleTokenAuthProof
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxOrder(t, nil, tokens.TransactionTypeSplitFT, nil),
			attr:       &tokens.SplitFungibleTokenAttributes{},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxOrder(t, existingTokenTypeID, tokens.TransactionTypeSplitFT, nil),
			attr:       &tokens.SplitFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxOrder(t, ftUnitID, tokens.TransactionTypeSplitFT, nil),
			attr:       &tokens.SplitFungibleTokenAttributes{},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", ftUnitID),
		},
		{
			name: "target value exceeds token value",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeSplitFT, nil),
			attr: &tokens.SplitFungibleTokenAttributes{
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       existingTokenValue + 1,
				Counter:           0,
			},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: "the target value must be less than the value of the source token: targetValue=1001 tokenValue=1000",
		},
		{
			name: "target value equals token value",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeSplitFT, nil),
			attr: &tokens.SplitFungibleTokenAttributes{
				TypeID:            existingTokenTypeID,
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       existingTokenValue,
				Counter:           0,
			},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: `the target value must be less than the value of the source token: targetValue=1000 tokenValue=1000`,
		},
		{
			name: "invalid value - target value is zero",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeSplitFT, nil),
			attr: &tokens.SplitFungibleTokenAttributes{
				TypeID:            existingTokenTypeID,
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       0,
				Counter:           0,
			},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: `when splitting a token the value assigned to the new token must be greater than zero`,
		},
		{
			name: "invalid counter",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeSplitFT, nil),
			attr: &tokens.SplitFungibleTokenAttributes{
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       existingTokenValue - 1,
				Counter:           1,
			},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: "invalid counter",
		},
		{
			name: "empty token type id",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeSplitFT, nil),
			attr: &tokens.SplitFungibleTokenAttributes{
				TypeID:            nil,
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       existingTokenValue - 1,
				Counter:           0,
			},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token type id",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeSplitFT, nil),
			attr: &tokens.SplitFungibleTokenAttributes{
				TypeID:            existingTokenTypeID2,
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       existingTokenValue - 1,
				Counter:           0,
			},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: "invalid type identifier",
		},
	}

	m, err := NewFungibleTokensModule(tokenid.PDR(), defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateSplitFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestSplitFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	pdr := tokenid.PDR()

	var remainingBillValue uint64 = 10
	attr := &tokens.SplitFungibleTokenAttributes{
		TypeID:            existingTokenTypeID,
		NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
		TargetValue:       existingTokenValue - remainingBillValue,
		Counter:           0,
	}
	authProof := &tokens.SplitFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}}
	uID := existingTokenID
	tx := createTxOrder(t, uID, tokens.TransactionTypeSplitFT, attr, testtransaction.WithAuthProof(authProof))
	var roundNo uint64 = 10
	m, err := NewFungibleTokensModule(pdr, opts)
	require.NoError(t, err)
	require.NoError(t, m.validateSplitFT(tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo))))
	sm, err := m.executeSplitFT(tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)

	require.EqualValues(t, templates.AlwaysTrueBytes(), d.Owner())
	require.Equal(t, remainingBillValue, d.Value)
	require.Equal(t, uint64(1), d.Counter)

	newUnitID, err := pdr.ComposeUnitID(types.ShardID{}, tokens.FungibleTokenUnitType, tokens.PrndSh(tx))
	require.NoError(t, err)
	newUnit, err := opts.state.GetUnit(newUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, newUnit)
	require.IsType(t, &tokens.FungibleTokenData{}, newUnit.Data())

	newUnitData := newUnit.Data().(*tokens.FungibleTokenData)

	require.Equal(t, attr.NewOwnerPredicate, newUnitData.Owner())
	require.Equal(t, existingTokenValue-remainingBillValue, newUnitData.Value)
	require.Equal(t, uint64(0), newUnitData.Counter)
}

func TestBurnFungibleToken_NotOk(t *testing.T) {
	ftUnitID := tokenid.NewFungibleTokenID(t)
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.BurnFungibleTokenAttributes
		authProof  *tokens.BurnFungibleTokenAuthProof
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxOrder(t, nil, tokens.TransactionTypeBurnFT, nil),
			attr:       &tokens.BurnFungibleTokenAttributes{},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxOrder(t, existingTokenTypeID, tokens.TransactionTypeBurnFT, nil),
			attr:       &tokens.BurnFungibleTokenAttributes{},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxOrder(t, ftUnitID, tokens.TransactionTypeBurnFT, nil),
			attr:       &tokens.BurnFungibleTokenAttributes{},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", ftUnitID),
		},
		{
			name: "invalid value",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeBurnFT, nil),
			attr: &tokens.BurnFungibleTokenAttributes{
				TypeID:             existingTokenTypeID,
				Value:              existingTokenValue - 1,
				TargetTokenCounter: 0,
				Counter:            0,
			},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid counter",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeBurnFT, nil),
			attr: &tokens.BurnFungibleTokenAttributes{
				TypeID:             existingTokenTypeID,
				Value:              existingTokenValue,
				TargetTokenCounter: 0,
				Counter:            1,
			},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: "invalid counter",
		},
		{
			name: "invalid token type",
			tx:   createTxOrder(t, existingTokenID, tokens.TransactionTypeBurnFT, nil),
			attr: &tokens.BurnFungibleTokenAttributes{
				TypeID: func() []byte {
					return tokenid.NewFungibleTokenTypeID(t)
				}(),
				Value:              existingTokenValue,
				TargetTokenCounter: 0,
				Counter:            0,
			},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: "type of token to burn does not matches the actual type of the token",
		},
	}

	m, err := NewFungibleTokensModule(tokenid.PDR(), defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateBurnFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestBurnFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	burnAttributes := &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenCounter: 0,
		Counter:            0,
	}
	authProof := &tokens.BurnFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}}
	uID := existingTokenID
	tx := createTxOrder(t, uID, tokens.TransactionTypeBurnFT, burnAttributes, testtransaction.WithAuthProof(authProof))
	roundNo := uint64(10)

	m, err := NewFungibleTokensModule(tokenid.PDR(), opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	// handle tx
	sm, err := txExecutors.ValidateAndExecute(tx, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	// get unit
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	// verify unit data
	d, ok := u.Data().(*tokens.FungibleTokenData)
	require.True(t, ok)
	require.EqualValues(t, templates.AlwaysFalseBytes(), d.Owner())
	require.Equal(t, existingTokenTypeID, d.TypeID)
	require.Equal(t, uint64(0), d.Value)
	require.Equal(t, uint64(1), d.Counter)
}

func TestJoinFungibleToken_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)
	m, err := NewFungibleTokensModule(tokenid.PDR(), opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	// burn existingTokenID and join to existingTokenID3
	burnAttributes := &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      existingTokenID3,
		TargetTokenCounter: 0,
		Counter:            0,
	}
	authProof := &tokens.BurnFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}}
	burnTxRecord := createTxRecord(t, existingTokenID, tokens.TransactionTypeBurnFT, burnAttributes, testtransaction.WithAuthProof(authProof))
	roundNo := uint64(10)
	sm, err := txExecutors.ValidateAndExecute(testtransaction.FetchTxoV1(t, burnTxRecord), testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	burnTxRecordProof := testblock.CreateTxRecordProof(t, burnTxRecord, signer, testblock.WithPartitionID(tokens.DefaultPartitionID))
	joinAttr := &tokens.JoinFungibleTokenAttributes{
		BurnTokenProofs: []*types.TxRecordProof{burnTxRecordProof},
	}
	burnAuthProof := testtransaction.WithAuthProof(&tokens.BurnFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}})
	joinTx := createTxOrder(t, existingTokenID3, tokens.TransactionTypeJoinFT, joinAttr, burnAuthProof)

	sm, err = txExecutors.ValidateAndExecute(joinTx, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
}

func TestJoinFungibleToken_NotOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)

	burnTxInvalidTargetTokenID := createTxRecord(t, existingTokenID, tokens.TransactionTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      test.RandomBytes(32),
		TargetTokenCounter: 0,
		Counter:            0,
	})
	burnTxInvalidTargetTokenCounter := createTxRecord(t, existingTokenID, tokens.TransactionTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      existingTokenID,
		TargetTokenCounter: 1,
		Counter:            0,
	})
	burnTx1 := createTxRecord(t, existingTokenID, tokens.TransactionTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      existingTokenID,
		TargetTokenCounter: 0,
		Counter:            0,
	})
	burnTx2 := createTxRecord(t, existingTokenID2, tokens.TransactionTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID2,
		Value:              existingTokenValue,
		TargetTokenID:      existingTokenID2,
		TargetTokenCounter: 0,
		Counter:            0,
	})
	maxUintValueTokenID := tokenid.NewFungibleTokenID(t)
	burnTx3 := createTxRecord(t, maxUintValueTokenID, tokens.TransactionTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID2,
		Value:              math.MaxUint64,
		TargetTokenID:      maxUintValueTokenID,
		TargetTokenCounter: 0,
		Counter:            0,
	})
	invalidNetworkID := createTxRecord(t, existingTokenID, tokens.TransactionTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      test.RandomBytes(32),
		TargetTokenCounter: 0,
		Counter:            0,
	}, testtransaction.WithNetworkID(10))
	invalidPartitionID := createTxRecord(t, existingTokenID, tokens.TransactionTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      test.RandomBytes(32),
		TargetTokenCounter: 0,
		Counter:            0,
	}, testtransaction.WithPartitionID(10))
	proofInvalidTargetTokenID := testblock.CreateTxRecordProof(t, burnTxInvalidTargetTokenID, signer)
	proofInvalidTargetTokenCounter := testblock.CreateTxRecordProof(t, burnTxInvalidTargetTokenCounter, signer)
	proofBurnTx2 := testblock.CreateTxRecordProof(t, burnTx2, signer)
	proofBurnTx3 := testblock.CreateTxRecordProof(t, burnTx3, signer)

	// create block with 3 burn txs
	pdr := tokenid.PDR()
	var burnTxRecordProofs []*types.TxRecordProof
	for i := uint8(3); i >= 1; i-- {
		unitID, err := pdr.ComposeUnitID(types.ShardID{}, tokens.FungibleTokenUnitType, func(b []byte) error { b[len(b)-1] = i; return nil })
		require.NoError(t, err)
		burnAuthProof := &tokens.BurnFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}}
		burnTxRecord := createTxRecord(t,
			unitID,
			tokens.TransactionTypeBurnFT,
			&tokens.BurnFungibleTokenAttributes{
				TypeID:             existingTokenTypeID,
				Value:              existingTokenValue,
				TargetTokenID:      existingTokenID,
				TargetTokenCounter: 0,
				Counter:            0,
			},
			testtransaction.WithAuthProof(burnAuthProof),
		)
		burnTxRecordProof := testblock.CreateTxRecordProof(t, burnTxRecord, signer, testblock.WithPartitionID(tokens.DefaultPartitionID))
		burnTxRecordProofs = append(burnTxRecordProofs, burnTxRecordProof)
	}

	ftUnitID := tokenid.NewFungibleTokenID(t)
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxOrder(t, nil, tokens.TransactionTypeJoinFT, &tokens.JoinFungibleTokenAttributes{}),
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxOrder(t, existingTokenTypeID, tokens.TransactionTypeJoinFT, &tokens.JoinFungibleTokenAttributes{}),
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxOrder(t, ftUnitID, tokens.TransactionTypeJoinFT, &tokens.JoinFungibleTokenAttributes{}),
			wantErrStr: fmt.Sprintf("unit %s does not exist", ftUnitID),
		},
		{
			name: "token identifiers in wrong order",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeJoinFT,
				&tokens.JoinFungibleTokenAttributes{BurnTokenProofs: burnTxRecordProofs},
				testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "burn transaction orders are not listed in strictly increasing order of token identifiers",
		},
		{
			name: "source not burned - invalid target token network id",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeJoinFT, &tokens.JoinFungibleTokenAttributes{
				BurnTokenProofs: []*types.TxRecordProof{
					{TxRecord: invalidNetworkID, TxProof: proofInvalidTargetTokenID.TxProof},
				},
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "burn transaction network id does not match with join transaction network id",
		},
		{
			name: "source not burned - invalid target token partition id",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeJoinFT, &tokens.JoinFungibleTokenAttributes{
				BurnTokenProofs: []*types.TxRecordProof{
					{TxRecord: invalidPartitionID, TxProof: proofInvalidTargetTokenID.TxProof},
				},
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "burn transaction partition id does not match with join transaction partition id",
		},
		{
			name: "source not burned - invalid target token id",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeJoinFT, &tokens.JoinFungibleTokenAttributes{
				BurnTokenProofs: []*types.TxRecordProof{proofInvalidTargetTokenID},
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "burn transaction target token id does not match with join transaction unit id",
		},
		{
			name: "source not burned - invalid target token counter",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeJoinFT, &tokens.JoinFungibleTokenAttributes{
				BurnTokenProofs: []*types.TxRecordProof{proofInvalidTargetTokenCounter},
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "burn transaction target token counter does not match with target unit counter",
		},
		{
			name: "invalid source token type",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeJoinFT, &tokens.JoinFungibleTokenAttributes{
				BurnTokenProofs: []*types.TxRecordProof{{
					TxRecord: burnTx2,
					TxProof:  proofInvalidTargetTokenID.TxProof,
				}},
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "the type of the burned source token does not match the type of target token",
		},
		{
			name: "proof is not valid",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeBurnFT, &tokens.JoinFungibleTokenAttributes{
				BurnTokenProofs: []*types.TxRecordProof{{
					TxRecord: burnTx1,
					TxProof:  proofBurnTx2.TxProof,
				}},
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "proof is not valid",
		},
		{
			name: "uint64 overflow",
			tx: createTxOrder(t, existingTokenID2, tokens.TransactionTypeBurnFT, &tokens.JoinFungibleTokenAttributes{
				BurnTokenProofs: []*types.TxRecordProof{proofBurnTx3},
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "invalid sum of tokens: uint64 overflow",
		},
	}

	m, err := NewFungibleTokensModule(tokenid.PDR(), opts)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.JoinFungibleTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))
			authProof := &tokens.JoinFungibleTokenAuthProof{}
			require.NoError(t, tt.tx.UnmarshalAuthProof(authProof))

			err := m.validateJoinFT(tt.tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func defaultOpts(t *testing.T) *Options {
	o, err := defaultOptions(observability.Default(t))
	require.NoError(t, err)
	o.state = initState(t)
	return o
}

func initState(t *testing.T) *state.State {
	s := state.NewEmptyState()
	err := s.Apply(state.AddUnit(existingTokenTypeID, &tokens.FungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		ParentTypeID:             nil,
		DecimalPlaces:            5,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingTokenTypeID2, &tokens.FungibleTokenTypeData{
		Symbol:                   "ALPHA2",
		Name:                     "A long name for ALPHA2",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		ParentTypeID:             nil,
		DecimalPlaces:            5,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingTokenID, &tokens.FungibleTokenData{
		TypeID:         existingTokenTypeID,
		Value:          existingTokenValue,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Counter:        0,
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingTokenID2, &tokens.FungibleTokenData{
		TypeID:         existingTokenTypeID2,
		Value:          existingTokenValue,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Counter:        0,
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingTokenID3, &tokens.FungibleTokenData{
		TypeID:         existingTokenTypeID,
		Value:          existingTokenValue,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Counter:        0,
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(feeCreditID, &fc.FeeCreditRecord{
		Balance:        100,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Counter:        10,
		MinLifetime:    100,
	}))
	require.NoError(t, err)
	return s
}

func createTxRecord(t *testing.T, unitID types.UnitID, transactionType uint16, attr any, opts ...testtransaction.Option) *types.TransactionRecord {
	txo := createTxOrder(t, unitID, transactionType, attr, opts...)
	return &types.TransactionRecord{
		Version:          1,
		TransactionOrder: testtransaction.TxoToBytes(t, txo),
		ServerMetadata: &types.ServerMetadata{
			ActualFee:        1,
			TargetUnits:      []types.UnitID{txo.UnitID},
			SuccessIndicator: types.TxStatusSuccessful,
		},
	}
}

func createTxOrder(t *testing.T, unitID types.UnitID, transactionType uint16, attr any, opts ...testtransaction.Option) *types.TransactionOrder {
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithTransactionType(transactionType),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPartitionID(tokens.DefaultPartitionID),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithAuthProof(defaultAuthProof(transactionType)),
	)
	for _, opt := range opts {
		require.NoError(t, opt(txo))
	}
	return txo
}

func defaultAuthProof(transactionType uint16) any {
	switch transactionType {
	case tokens.TransactionTypeDefineNFT:
		return &tokens.DefineNonFungibleTokenAuthProof{}
	case tokens.TransactionTypeMintNFT:
		return &tokens.MintNonFungibleTokenAuthProof{}
	case tokens.TransactionTypeTransferNFT:
		return &tokens.TransferNonFungibleTokenAuthProof{}
	case tokens.TransactionTypeUpdateNFT:
		return &tokens.UpdateNonFungibleTokenAuthProof{}
	case tokens.TransactionTypeDefineFT:
		return &tokens.DefineFungibleTokenAuthProof{}
	case tokens.TransactionTypeMintFT:
		return &tokens.MintFungibleTokenAuthProof{}
	case tokens.TransactionTypeTransferFT:
		return &tokens.TransferFungibleTokenAuthProof{}
	case tokens.TransactionTypeSplitFT:
		return &tokens.SplitFungibleTokenAuthProof{}
	case tokens.TransactionTypeBurnFT:
		return &tokens.BurnFungibleTokenAuthProof{}
	case tokens.TransactionTypeJoinFT:
		return &tokens.JoinFungibleTokenAuthProof{}
	default:
		return nil
	}
}
