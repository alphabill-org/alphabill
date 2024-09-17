package tokens

import (
	"fmt"
	"math"
	"testing"

	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
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
	existingTokenTypeID   = tokens.NewFungibleTokenTypeID(nil, []byte{1})
	existingTokenTypeID2  = tokens.NewFungibleTokenTypeID(nil, []byte{1, 0, 0, 1})
	existingTokenID       = tokens.NewFungibleTokenID(nil, []byte{0x02})
	existingTokenID2      = tokens.NewFungibleTokenID(nil, []byte{0xaa})
	existingLockedTokenID = tokens.NewFungibleTokenID(nil, []byte{0xbb})

	nonExistingTokenTypeID = tokens.NewFungibleTokenTypeID(nil, []byte{100})
	nonExistingTokenID     = tokens.NewFungibleTokenID(nil, []byte{100})

	invalidFungibleTokenTypeID = tokens.NewNonFungibleTokenTypeID(nil, []byte{0x02}) // use non-fungible type id
	invalidFungibleTokenID     = tokens.NewNonFungibleTokenID(nil, []byte{1})        // use non-fungible type id
)

func TestCreateFungibleTokenType_NotOk(t *testing.T) {
	unitID, err := tokens.NewRandomFungibleTokenTypeID(nil)
	require.NoError(t, err)

	validTxOrder := &types.TransactionOrder{
		Payload: &types.Payload{
			Type:   tokens.PayloadTypeDefineFT,
			UnitID: unitID,
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
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeDefineFT,
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
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeDefineFT,
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
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeDefineFT,
					UnitID: existingTokenTypeID,
				},
			},
			attr:       &tokens.DefineFungibleTokenAttributes{ParentTypeID: existingTokenID},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: "invalid parent type ID",
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
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeDefineFT,
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
			attr:       &tokens.DefineFungibleTokenAttributes{Symbol: validSymbol, DecimalPlaces: 6, ParentTypeID: unitID},
			authProof:  &tokens.DefineFungibleTokenAuthProof{},
			options:    defaultOpts(t),
			wantErrStr: fmt.Sprintf("item %s does not exist", unitID),
		},
	}

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = m.validateDefineFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
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
	unitID := tokens.NewFungibleTokenTypeID(nil, []byte{7})

	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	txo := createTxo(t, unitID, tokens.PayloadTypeDefineFT, attributes)
	sm, err := txExecutors.ValidateAndExecute(
		txo,
		testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)),
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
	require.Equal(t, attributes.SubTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.Equal(t, attributes.TokenMintingPredicate, d.TokenMintingPredicate)
	require.Equal(t, attributes.TokenTypeOwnerPredicate, d.TokenTypeOwnerPredicate)
	require.Nil(t, d.ParentTypeID)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_Ok(t *testing.T) {
	opts := defaultOpts(t)

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
	parentID := tokens.NewFungibleTokenTypeID(nil, []byte{19})
	parentTx := createTxo(t, parentID, tokens.PayloadTypeDefineFT, parentAttributes)

	childID := tokens.NewFungibleTokenTypeID(nil, []byte{20})
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
	childTx := createTxo(t, childID, tokens.PayloadTypeDefineFT, childAttributes, testtransaction.WithAuthProof(tokens.DefineFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{templates.EmptyArgument()}}))

	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	exeCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))
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
	require.Equal(t, childAttributes.SubTypeCreationPredicate, d.SubTypeCreationPredicate)
	require.Equal(t, childAttributes.TokenMintingPredicate, d.TokenMintingPredicate)
	require.Equal(t, childAttributes.TokenTypeOwnerPredicate, d.TokenTypeOwnerPredicate)
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
	parentID := tokens.NewFungibleTokenTypeID(nil, []byte{19})
	parentTx := createTxo(t, parentID, tokens.PayloadTypeDefineFT, parentAttributes)

	childID := tokens.NewFungibleTokenTypeID(nil, []byte{20})
	childAttributes := &tokens.DefineFungibleTokenAttributes{
		Symbol:                   validSymbol + "_CHILD",
		ParentTypeID:             parentID,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.AlwaysFalseBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
	}
	childTx := createTxo(t, childID, tokens.PayloadTypeDefineFT, childAttributes, testtransaction.WithAuthProof(tokens.DefineFungibleTokenAuthProof{SubTypeCreationProofs: [][]byte{[]byte("invalid")}}))
	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	sm, err := txExecutors.ValidateAndExecute(parentTx, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	sm, err = txExecutors.ValidateAndExecute(childTx, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(112)))
	require.EqualError(t, err, `'defFT' validation failed: SubTypeCreationPredicate: executing predicate [0] in the chain: executing predicate: failed to decode P2PKH256 signature: unexpected EOF`)
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
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFT,
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
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFT,
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
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFT,
					UnitID: existingTokenID,
				},
			},
			attr:       &tokens.MintFungibleTokenAttributes{TypeID: invalidFungibleTokenTypeID},
			authProof:  &tokens.MintFungibleTokenAuthProof{},
			wantErrStr: "invalid token type ID",
		},
		{
			name: "unit type with given ID does not exist",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFT,
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
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFT,
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
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFT,
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

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateMintFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestMintFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	attributes := &tokens.MintFungibleTokenAttributes{
		OwnerPredicate: templates.AlwaysTrueBytes(),
		TypeID:         existingTokenTypeID,
		Value:          existingTokenValue,
	}
	tx := createTxo(t, nil, tokens.PayloadTypeMintFT, attributes)
	tx.Payload.UnitID = newFungibleTokenID(t, tx)
	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	sm, err := txExecutors.ValidateAndExecute(tx, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := opts.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())

	d := u.Data().(*tokens.FungibleTokenData)
	require.Equal(t, existingTokenTypeID, d.TokenType)
	require.Equal(t, attributes.Value, d.Value)
	require.Equal(t, uint64(10), d.T)
	require.Equal(t, uint64(0), d.Counter)
	require.Equal(t, uint64(1000), d.T1)
	require.Equal(t, uint64(0), d.Locked)
	require.Equal(t, attributes.OwnerPredicate, []byte(u.Owner()))
}

func TestTransferFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.TransferFungibleTokenAttributes
		authProof  *tokens.TransferFungibleTokenAuthProof
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxo(t, nil, tokens.PayloadTypeTransferFT, nil),
			attr:       &tokens.TransferFungibleTokenAttributes{},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxo(t, existingTokenTypeID, tokens.PayloadTypeTransferFT, nil),
			attr:       &tokens.TransferFungibleTokenAttributes{},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxo(t, tokens.NewFungibleTokenID(nil, []byte{42}), tokens.PayloadTypeTransferFT, nil),
			attr:       &tokens.TransferFungibleTokenAttributes{},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token locked",
			tx:   createTxo(t, existingLockedTokenID, tokens.PayloadTypeTransferFT, &tokens.TransferFungibleTokenAttributes{}),
			attr: &tokens.TransferFungibleTokenAttributes{
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				Value:             existingTokenValue,
				Counter:           0,
			},
			authProof:  &tokens.TransferFungibleTokenAuthProof{},
			wantErrStr: "token is locked",
		},
		{
			name: "invalid value",
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeTransferFT, &tokens.TransferFungibleTokenAttributes{}),
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
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeTransferFT, &tokens.TransferFungibleTokenAttributes{}),
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
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeTransferFT, &tokens.TransferFungibleTokenAttributes{}),
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
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeTransferFT, &tokens.TransferFungibleTokenAttributes{}),
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

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateTransferFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
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
	tx := createTxo(t, uID, tokens.PayloadTypeTransferFT, transferAttributes, testtransaction.WithAuthProof(authProof))
	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	var roundNo uint64 = 10
	require.NoError(t, m.validateTransferFT(tx, transferAttributes, authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo))))
	sm, err := m.executeTransferFT(tx, transferAttributes, authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)

	require.Equal(t, transferAttributes.NewOwnerPredicate, []byte(u.Owner()))
	require.Equal(t, transferAttributes.Value, d.Value)
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, roundNo, d.T)
}

func TestSplitFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.SplitFungibleTokenAttributes
		authProof  *tokens.SplitFungibleTokenAuthProof
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxo(t, nil, tokens.PayloadTypeSplitFT, nil),
			attr:       &tokens.SplitFungibleTokenAttributes{},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxo(t, existingTokenTypeID, tokens.PayloadTypeSplitFT, nil),
			attr:       &tokens.SplitFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxo(t, tokens.NewFungibleTokenID(nil, []byte{42}), tokens.PayloadTypeSplitFT, nil),
			attr:       &tokens.SplitFungibleTokenAttributes{},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token locked",
			tx:   createTxo(t, existingLockedTokenID, tokens.PayloadTypeSplitFT, nil),
			attr: &tokens.SplitFungibleTokenAttributes{
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       existingTokenValue,
				Counter:           0,
			},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: "token is locked",
		},
		{
			name: "target value exceeds token value",
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeSplitFT, nil),
			attr: &tokens.SplitFungibleTokenAttributes{
				NewOwnerPredicate: templates.AlwaysTrueBytes(),
				TargetValue:       existingTokenValue + 1,
				Counter:           0,
			},
			authProof:  &tokens.SplitFungibleTokenAuthProof{},
			wantErrStr: fmt.Sprintf("the target value must be less than the value of the source token: targetValue=1001 tokenValue=1000"),
		},
		{
			name: "target value equals token value",
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeSplitFT, nil),
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
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeSplitFT, nil),
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
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeSplitFT, nil),
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
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeSplitFT, nil),
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
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeSplitFT, nil),
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

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateSplitFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestSplitFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)

	var remainingBillValue uint64 = 10
	attr := &tokens.SplitFungibleTokenAttributes{
		TypeID:            existingTokenTypeID,
		NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
		TargetValue:       existingTokenValue - remainingBillValue,
		Counter:           0,
	}
	authProof := &tokens.SplitFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}}
	uID := existingTokenID
	tx := createTxo(t, uID, tokens.PayloadTypeSplitFT, attr, testtransaction.WithAuthProof(authProof))
	var roundNo uint64 = 10
	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	require.NoError(t, m.validateSplitFT(tx, attr, authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo))))
	sm, err := m.executeSplitFT(tx, attr, authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)

	require.EqualValues(t, templates.AlwaysTrueBytes(), []byte(u.Owner()))
	require.Equal(t, remainingBillValue, d.Value)
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, roundNo, d.T)

	newUnitID := tokens.NewFungibleTokenID(uID, tx.HashForNewUnitID(opts.hashAlgorithm))
	newUnit, err := opts.state.GetUnit(newUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, newUnit)
	require.IsType(t, &tokens.FungibleTokenData{}, newUnit.Data())

	newUnitData := newUnit.Data().(*tokens.FungibleTokenData)

	require.Equal(t, attr.NewOwnerPredicate, []byte(newUnit.Owner()))
	require.Equal(t, existingTokenValue-remainingBillValue, newUnitData.Value)
	require.Equal(t, uint64(0), newUnitData.Counter)
	require.Equal(t, roundNo, newUnitData.T)
}

func TestBurnFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.BurnFungibleTokenAttributes
		authProof  *tokens.BurnFungibleTokenAuthProof
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxo(t, nil, tokens.PayloadTypeBurnFT, nil),
			attr:       &tokens.BurnFungibleTokenAttributes{},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxo(t, existingTokenTypeID, tokens.PayloadTypeBurnFT, nil),
			attr:       &tokens.BurnFungibleTokenAttributes{},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxo(t, tokens.NewFungibleTokenID(nil, []byte{42}), tokens.PayloadTypeBurnFT, nil),
			attr:       &tokens.BurnFungibleTokenAttributes{},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token locked",
			tx:   createTxo(t, existingLockedTokenID, tokens.PayloadTypeBurnFT, nil),
			attr: &tokens.BurnFungibleTokenAttributes{
				TypeID:             existingTokenTypeID,
				Value:              existingTokenValue,
				TargetTokenCounter: 0,
				Counter:            0,
			},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: "token is locked",
		},
		{
			name: "invalid value",
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeBurnFT, nil),
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
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeBurnFT, nil),
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
			tx:   createTxo(t, existingTokenID, tokens.PayloadTypeBurnFT, nil),
			attr: &tokens.BurnFungibleTokenAttributes{
				TypeID: func() []byte {
					r := tokens.NewFungibleTokenTypeID(nil, []byte{42})
					return r[:]
				}(),
				Value:              existingTokenValue,
				TargetTokenCounter: 0,
				Counter:            0,
			},
			authProof:  &tokens.BurnFungibleTokenAuthProof{},
			wantErrStr: "type of token to burn does not matches the actual type of the token",
		},
	}

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateBurnFT(tt.tx, tt.attr, tt.authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
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
	tx := createTxo(t, uID, tokens.PayloadTypeBurnFT, burnAttributes, testtransaction.WithAuthProof(authProof))
	roundNo := uint64(10)

	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	// handle tx
	sm, err := txExecutors.ValidateAndExecute(tx, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	// get unit
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	// verify owner is dc predicate
	require.Equal(t, templates.AlwaysFalseBytes(), u.Owner())

	// verify unit data
	unitData, ok := u.Data().(*tokens.FungibleTokenData)
	require.True(t, ok)
	require.Equal(t, existingTokenTypeID, unitData.TokenType)
	require.Equal(t, uint64(0), unitData.Value)
	require.Equal(t, roundNo, unitData.T)
	require.Equal(t, uint64(1), unitData.Counter)
	require.Equal(t, uint64(0), unitData.Locked)
}

func TestJoinFungibleToken_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)
	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	burnAttributes := &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      existingLockedTokenID,
		TargetTokenCounter: 0,
		Counter:            0,
	}
	authProof := &tokens.BurnFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}}
	burnTx := createTxr(t, existingTokenID, tokens.PayloadTypeBurnFT, burnAttributes, testtransaction.WithAuthProof(authProof))
	roundNo := uint64(10)
	sm, err := txExecutors.ValidateAndExecute(burnTx.TransactionOrder, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	burnTxProof := testblock.CreateProof(t, burnTx, signer, testblock.WithSystemIdentifier(tokens.DefaultSystemID))
	joinAttr := &tokens.JoinFungibleTokenAttributes{
		BurnTransactions: []*types.TransactionRecord{burnTx},
		Proofs:           []*types.TxProof{burnTxProof},
		Counter:          0,
	}
	burnAuthProof := testtransaction.WithAuthProof(&tokens.BurnFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}})
	joinTx := createTxo(t, existingLockedTokenID, tokens.PayloadTypeJoinFT, joinAttr, burnAuthProof)

	sm, err = txExecutors.ValidateAndExecute(joinTx, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify locked target unit was unlocked
	u, err := opts.state.GetUnit(existingLockedTokenID, false)
	require.NoError(t, err)
	require.EqualValues(t, 0, u.Data().(*tokens.FungibleTokenData).Locked)
}

func TestJoinFungibleToken_NotOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)

	burnTxInvalidTargetTokenID := createTxr(t, existingTokenID, tokens.PayloadTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      test.RandomBytes(32),
		TargetTokenCounter: 0,
		Counter:            0,
	})
	burnTxInvalidTargetTokenCounter := createTxr(t, existingTokenID, tokens.PayloadTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      existingTokenID,
		TargetTokenCounter: 0,
		Counter:            0,
	})
	burnTx1 := createTxr(t, existingTokenID, tokens.PayloadTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID,
		Value:              existingTokenValue,
		TargetTokenID:      existingTokenID,
		TargetTokenCounter: 0,
		Counter:            0,
	})
	burnTx2 := createTxr(t, existingTokenID2, tokens.PayloadTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID2,
		Value:              existingTokenValue,
		TargetTokenID:      existingTokenID2,
		TargetTokenCounter: 0,
		Counter:            0,
	})
	maxUintValueTokenID := tokens.NewFungibleTokenID(nil, []byte{1, 0, 0, 2})
	burnTx3 := createTxr(t, maxUintValueTokenID, tokens.PayloadTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
		TypeID:             existingTokenTypeID2,
		Value:              math.MaxUint64,
		TargetTokenID:      maxUintValueTokenID,
		TargetTokenCounter: 0,
		Counter:            0,
	})
	proofInvalidTargetTokenID := testblock.CreateProof(t, burnTxInvalidTargetTokenID, signer)
	proofInvalidTargetTokenCounter := testblock.CreateProof(t, burnTxInvalidTargetTokenCounter, signer)
	proofBurnTx2 := testblock.CreateProof(t, burnTx2, signer)
	proofBurnTx3 := testblock.CreateProof(t, burnTx3, signer)

	// create block with 3 burn txs
	var burnTxs []*types.TransactionRecord
	for i := uint8(3); i >= 1; i-- {
		burnAuthProof := &tokens.BurnFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.EmptyArgument()}}
		burnTx := createTxr(t, tokens.NewFungibleTokenID(nil, []byte{i}), tokens.PayloadTypeBurnFT, &tokens.BurnFungibleTokenAttributes{
			TypeID:             existingTokenTypeID,
			Value:              existingTokenValue,
			TargetTokenID:      existingTokenID,
			TargetTokenCounter: 0,
			Counter:            0,
		}, testtransaction.WithAuthProof(burnAuthProof))
		burnTxs = append(burnTxs, burnTx)
	}
	proofs := testblock.CreateProofs(t, burnTxs, signer, testblock.WithSystemIdentifier(tokens.DefaultSystemID))

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxo(t, nil, tokens.PayloadTypeJoinFT, &tokens.JoinFungibleTokenAttributes{}),
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxo(t, existingTokenTypeID, tokens.PayloadTypeJoinFT, &tokens.JoinFungibleTokenAttributes{}),
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxo(t, tokens.NewFungibleTokenID(nil, []byte{42}), tokens.PayloadTypeJoinFT, &tokens.JoinFungibleTokenAttributes{}),
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name:       "invalid counter",
			tx:         createTxo(t, existingTokenID, tokens.PayloadTypeJoinFT, &tokens.JoinFungibleTokenAttributes{Counter: 1}),
			wantErrStr: "invalid counter",
		},
		{
			name: "token identifiers in wrong order",
			tx: createTxo(t, existingTokenID, tokens.PayloadTypeJoinFT, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions: burnTxs,
				Proofs:           proofs,
				Counter:          0,
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "burn transaction orders are not listed in strictly increasing order of token identifiers",
		},
		{
			name: "source not burned - invalid target token id",
			tx: createTxo(t, existingTokenID, tokens.PayloadTypeJoinFT, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions: []*types.TransactionRecord{burnTxInvalidTargetTokenID},
				Proofs:           []*types.TxProof{proofInvalidTargetTokenID},
				Counter:          0,
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "burn transaction target token id does not match with join transaction unit id",
		},
		{
			name: "source not burned - invalid target token counter",
			tx: createTxo(t, existingTokenID, tokens.PayloadTypeJoinFT, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions: []*types.TransactionRecord{burnTxInvalidTargetTokenCounter},
				Proofs:           []*types.TxProof{proofInvalidTargetTokenCounter},
				Counter:          1,
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "burn transaction target token counter does not match with join transaction counter",
		},
		{
			name: "invalid source token type",
			tx: createTxo(t, existingTokenID, tokens.PayloadTypeJoinFT, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions: []*types.TransactionRecord{burnTx2},
				Proofs:           []*types.TxProof{proofInvalidTargetTokenID},
				Counter:          0,
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "the type of the burned source token does not match the type of target token",
		},
		{
			name: "proof is not valid",
			tx: createTxo(t, existingTokenID, tokens.PayloadTypeBurnFT, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions: []*types.TransactionRecord{burnTx1},
				Proofs:           []*types.TxProof{proofBurnTx2},
				Counter:          0,
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "proof is not valid",
		},
		{
			name: "uint64 overflow",
			tx: createTxo(t, existingTokenID2, tokens.PayloadTypeBurnFT, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions: []*types.TransactionRecord{burnTx3},
				Proofs:           []*types.TxProof{proofBurnTx3},
				Counter:          0,
			}, testtransaction.WithAuthProof(tokens.JoinFungibleTokenAuthProof{TokenTypeOwnerProofs: [][]byte{templates.AlwaysFalseBytes()}})),
			wantErrStr: "invalid sum of tokens: uint64 overflow",
		},
	}

	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.JoinFungibleTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))
			authProof := &tokens.JoinFungibleTokenAuthProof{}
			require.NoError(t, tt.tx.UnmarshalAuthProof(authProof))

			err := m.validateJoinFT(tt.tx, attr, authProof, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func defaultOpts(t *testing.T) *Options {
	o, err := defaultOptions()
	require.NoError(t, err)
	o.state = initState(t)
	return o
}

func initState(t *testing.T) *state.State {
	s := state.NewEmptyState()
	err := s.Apply(state.AddUnit(existingTokenTypeID, templates.AlwaysTrueBytes(), &tokens.FungibleTokenTypeData{
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
	err = s.Apply(state.AddUnit(existingTokenTypeID2, templates.AlwaysTrueBytes(), &tokens.FungibleTokenTypeData{
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
	err = s.Apply(state.AddUnit(existingTokenID, templates.AlwaysTrueBytes(), &tokens.FungibleTokenData{
		TokenType: existingTokenTypeID,
		Value:     existingTokenValue,
		T:         0,
		Counter:   0,
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingTokenID2, templates.AlwaysTrueBytes(), &tokens.FungibleTokenData{
		TokenType: existingTokenTypeID2,
		Value:     existingTokenValue,
		T:         0,
		Counter:   0,
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingLockedTokenID, templates.AlwaysTrueBytes(), &tokens.FungibleTokenData{
		TokenType: existingTokenTypeID,
		Value:     existingTokenValue,
		T:         0,
		Counter:   0,
		Locked:    1,
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(feeCreditID, templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{
		Balance: 100,
		Counter: 10,
		Timeout: 100,
	}))
	require.NoError(t, err)
	return s
}

func createTxr(t *testing.T, unitID types.UnitID, payloadType string, attr any, opts ...testtransaction.Option) *types.TransactionRecord {
	txo := createTxo(t, unitID, payloadType, attr, opts...)
	return &types.TransactionRecord{
		TransactionOrder: txo,
		ServerMetadata: &types.ServerMetadata{
			ActualFee:        1,
			TargetUnits:      []types.UnitID{txo.UnitID()},
			SuccessIndicator: types.TxStatusSuccessful,
		},
	}
}

func createTxo(t *testing.T, unitID types.UnitID, payloadType string, attr any, opts ...testtransaction.Option) *types.TransactionOrder {
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithPayloadType(payloadType),
		testtransaction.WithAttributes(attr),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
		testtransaction.WithAuthProof(defaultAuthProof(payloadType)),
	)
	for _, opt := range opts {
		require.NoError(t, opt(txo))
	}
	return txo
}

func defaultAuthProof(payloadType string) any {
	switch payloadType {
	case tokens.PayloadTypeDefineNFT:
		return &tokens.DefineNonFungibleTokenAuthProof{}
	case tokens.PayloadTypeMintNFT:
		return &tokens.MintNonFungibleTokenAuthProof{}
	case tokens.PayloadTypeTransferNFT:
		return &tokens.TransferNonFungibleTokenAuthProof{}
	case tokens.PayloadTypeUpdateNFT:
		return &tokens.UpdateNonFungibleTokenAuthProof{}
	case tokens.PayloadTypeDefineFT:
		return &tokens.DefineFungibleTokenAuthProof{}
	case tokens.PayloadTypeMintFT:
		return &tokens.MintFungibleTokenAuthProof{}
	case tokens.PayloadTypeTransferFT:
		return &tokens.TransferFungibleTokenAuthProof{}
	case tokens.PayloadTypeSplitFT:
		return &tokens.SplitFungibleTokenAuthProof{}
	case tokens.PayloadTypeBurnFT:
		return &tokens.BurnFungibleTokenAuthProof{}
	case tokens.PayloadTypeJoinFT:
		return &tokens.JoinFungibleTokenAuthProof{}
	case tokens.PayloadTypeLockToken:
		return &tokens.LockTokenAuthProof{}
	case tokens.PayloadTypeUnlockToken:
		return &tokens.UnlockTokenAuthProof{}
	default:
		return nil
	}
}
