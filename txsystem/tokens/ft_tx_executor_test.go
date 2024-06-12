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
			Type:   tokens.PayloadTypeCreateFungibleTokenType,
			UnitID: unitID,
		},
	}
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.CreateFungibleTokenTypeAttributes
		options    *Options
		wantErrStr string
	}{
		{
			name: "nil unitID",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeCreateFungibleTokenType,
					UnitID: nil,
				},
			},
			attr:       &tokens.CreateFungibleTokenTypeAttributes{},
			options:    defaultOpts(t),
			wantErrStr: "invalid unit ID",
		},
		{
			name: "unitID has wrong type",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeCreateFungibleTokenType,
					UnitID: existingTokenID,
				},
			},
			attr:       &tokens.CreateFungibleTokenTypeAttributes{},
			options:    defaultOpts(t),
			wantErrStr: "invalid unit ID",
		},
		{
			name: "parentTypeID has wrong type",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeCreateFungibleTokenType,
					UnitID: existingTokenTypeID,
				},
			},
			attr:       &tokens.CreateFungibleTokenTypeAttributes{ParentTypeID: existingTokenID},
			options:    defaultOpts(t),
			wantErrStr: "invalid parent type ID",
		},
		{
			name:       "symbol length exceeds the allowed maximum",
			tx:         validTxOrder,
			attr:       &tokens.CreateFungibleTokenTypeAttributes{Symbol: invalidSymbol},
			options:    defaultOpts(t),
			wantErrStr: "symbol length exceeds the allowed maximum of 16 bytes",
		},
		{
			name:       "name length exceeds the allowed maximum",
			tx:         validTxOrder,
			attr:       &tokens.CreateFungibleTokenTypeAttributes{Name: invalidName},
			options:    defaultOpts(t),
			wantErrStr: "name length exceeds the allowed maximum of 256 bytes",
		},
		{
			name: "icon type length exceeds the allowed maximum",
			tx:   validTxOrder,
			attr: &tokens.CreateFungibleTokenTypeAttributes{
				Symbol: validSymbol,
				Icon:   &tokens.Icon{Type: invalidIconType, Data: test.RandomBytes(maxIconDataLength)},
			},
			options:    defaultOpts(t),
			wantErrStr: "icon type length exceeds the allowed maximum of 64 bytes",
		},
		{
			name: "icon data length exceeds the allowed maximum",

			tx: validTxOrder,
			attr: &tokens.CreateFungibleTokenTypeAttributes{
				Symbol: validSymbol,
				Icon:   &tokens.Icon{Type: validIconType, Data: test.RandomBytes(maxIconDataLength + 1)},
			},
			options:    defaultOpts(t),
			wantErrStr: "icon data length exceeds the allowed maximum of 64 KiB",
		},
		{
			name:       "decimal places > 8",
			tx:         validTxOrder,
			attr:       &tokens.CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 9},
			options:    defaultOpts(t),
			wantErrStr: "invalid decimal places. maximum allowed value 8, got 9",
		},
		{
			name: "unit with given ID exists",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeCreateFungibleTokenType,
					UnitID: existingTokenTypeID,
				},
			},
			attr:       &tokens.CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 5},
			options:    defaultOpts(t),
			wantErrStr: fmt.Sprintf("unit %s exists", existingTokenTypeID),
		},
		{
			name:       "parent.decimals != tx.attributes.decimalPlaces",
			tx:         validTxOrder,
			attr:       &tokens.CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 6, ParentTypeID: existingTokenTypeID},
			options:    defaultOpts(t),
			wantErrStr: "invalid decimal places. allowed 5, got 6",
		},
		{
			name:       "parent does not exist",
			tx:         validTxOrder,
			attr:       &tokens.CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 6, ParentTypeID: unitID},
			options:    defaultOpts(t),
			wantErrStr: fmt.Sprintf("item %s does not exist", unitID),
		},
	}

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = m.validateCreateFTType(tt.tx, tt.attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestCreateFungibleTokenType_CreateSingleType_Ok(t *testing.T) {
	opts := defaultOpts(t)
	attributes := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbol,
		Name:                     validName,
		Icon:                     &tokens.Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeID:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.AlwaysFalseBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
	}
	unitID := tokens.NewFungibleTokenTypeID(nil, []byte{7})

	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	sm, err := txExecutors.ValidateAndExecute(
		createTransactionOrder(t, attributes, tokens.PayloadTypeCreateFungibleTokenType, unitID),
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
	require.Equal(t, attributes.TokenCreationPredicate, d.TokenCreationPredicate)
	require.Equal(t, attributes.InvariantPredicate, d.InvariantPredicate)
	require.Nil(t, d.ParentTypeID)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_Ok(t *testing.T) {
	opts := defaultOpts(t)

	parentAttributes := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbol,
		Name:                     validName,
		Icon:                     &tokens.Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeID:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysFalseBytes(),
		InvariantPredicate:       templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
	}
	parentID := tokens.NewFungibleTokenTypeID(nil, []byte{19})
	parentTx := createTransactionOrder(t, parentAttributes, tokens.PayloadTypeCreateFungibleTokenType, parentID)

	childID := tokens.NewFungibleTokenTypeID(nil, []byte{20})
	childAttributes := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                             validSymbol + "_CHILD",
		Name:                               validName + "_CHILD",
		Icon:                               &tokens.Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeID:                       parentID,
		DecimalPlaces:                      6,
		SubTypeCreationPredicate:           templates.AlwaysFalseBytes(),
		TokenCreationPredicate:             templates.AlwaysTrueBytes(),
		InvariantPredicate:                 templates.AlwaysTrueBytes(),
		SubTypeCreationPredicateSignatures: [][]byte{nil},
	}

	childTx := createTransactionOrder(t, childAttributes, tokens.PayloadTypeCreateFungibleTokenType, childID)

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
	require.Equal(t, childAttributes.TokenCreationPredicate, d.TokenCreationPredicate)
	require.Equal(t, childAttributes.InvariantPredicate, d.InvariantPredicate)
	require.Equal(t, parentID, d.ParentTypeID)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_InvalidCreationPredicateSignature(t *testing.T) {
	opts := defaultOpts(t)

	parentAttributes := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbol,
		ParentTypeID:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
		TokenCreationPredicate:   templates.AlwaysFalseBytes(),
		InvariantPredicate:       templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
	}
	parentID := tokens.NewFungibleTokenTypeID(nil, []byte{19})
	parentTx := createTransactionOrder(t, parentAttributes, tokens.PayloadTypeCreateFungibleTokenType, parentID)

	childID := tokens.NewFungibleTokenTypeID(nil, []byte{20})
	childAttributes := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                             validSymbol + "_CHILD",
		ParentTypeID:                       parentID,
		DecimalPlaces:                      6,
		SubTypeCreationPredicate:           templates.AlwaysFalseBytes(),
		TokenCreationPredicate:             templates.AlwaysTrueBytes(),
		InvariantPredicate:                 templates.AlwaysTrueBytes(),
		SubTypeCreationPredicateSignatures: [][]byte{[]byte("invalid")},
	}
	childTx := createTransactionOrder(t, childAttributes, tokens.PayloadTypeCreateFungibleTokenType, childID)
	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	sm, err := txExecutors.ValidateAndExecute(parentTx, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	sm, err = txExecutors.ValidateAndExecute(childTx, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(112)))
	require.EqualError(t, err, `'createFType' validation failed: SubTypeCreationPredicate: executing predicate [0] in the chain: executing predicate: failed to decode P2PKH256 signature: unexpected EOF`)
	require.Nil(t, sm)
}

func TestMintFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.MintFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name: "unit ID is nil",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFungibleToken,
					UnitID: nil,
				},
			},
			attr:       &tokens.MintFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name: "unit ID has wrong type",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFungibleToken,
					UnitID: invalidFungibleTokenID,
				},
			},
			attr:       &tokens.MintFungibleTokenAttributes{TypeID: existingTokenTypeID},
			wantErrStr: "invalid unit ID",
		},
		{
			name: "type ID has wrong type",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFungibleToken,
					UnitID: existingTokenID,
				},
			},
			attr:       &tokens.MintFungibleTokenAttributes{TypeID: invalidFungibleTokenTypeID},
			wantErrStr: "invalid token type ID",
		},
		{
			name: "unit type with given ID does not exist",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFungibleToken,
					UnitID: nonExistingTokenID,
				},
			},
			attr:       &tokens.MintFungibleTokenAttributes{TypeID: nonExistingTokenTypeID},
			wantErrStr: "token type does not exist",
		},
		{
			name: "token type does not exist",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFungibleToken,
					UnitID: nonExistingTokenID,
				},
			},
			attr: &tokens.MintFungibleTokenAttributes{
				Bearer:                           templates.AlwaysTrueBytes(),
				TypeID:                           nonExistingTokenTypeID,
				Value:                            1000,
				TokenCreationPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: fmt.Sprintf("token type does not exist"),
		},
		{
			name: "invalid value - zero",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   tokens.PayloadTypeMintFungibleToken,
					UnitID: nonExistingTokenID,
				},
			},
			attr: &tokens.MintFungibleTokenAttributes{
				Bearer:                           templates.AlwaysTrueBytes(),
				TypeID:                           existingTokenTypeID,
				Value:                            0,
				TokenCreationPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: `token must have value greater than zero`,
		},
	}

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateMintFT(tt.tx, tt.attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestMintFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	attributes := &tokens.MintFungibleTokenAttributes{
		Bearer:                           templates.AlwaysTrueBytes(),
		TypeID:                           existingTokenTypeID,
		Value:                            1000,
		TokenCreationPredicateSignatures: [][]byte{nil},
	}
	tx := createTransactionOrder(t, attributes, tokens.PayloadTypeMintFungibleToken, nil)
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
	require.Equal(t, attributes.Bearer, []byte(u.Bearer()))
}

func TestTransferFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.TransferFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeTransferFungibleToken, nil),
			attr:       &tokens.TransferFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeTransferFungibleToken, existingTokenTypeID),
			attr:       &tokens.TransferFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeTransferFungibleToken, tokens.NewFungibleTokenID(nil, []byte{42})),
			attr:       &tokens.TransferFungibleTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token locked",
			tx:   createTransactionOrder(t, &tokens.TransferFungibleTokenAttributes{}, tokens.PayloadTypeTransferFungibleToken, existingLockedTokenID),
			attr: &tokens.TransferFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "token is locked",
		},
		{
			name: "invalid value",
			tx:   createTransactionOrder(t, &tokens.TransferFungibleTokenAttributes{}, tokens.PayloadTypeTransferFungibleToken, existingTokenID),
			attr: &tokens.TransferFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue - 1,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid counter",
			tx:   createTransactionOrder(t, &tokens.TransferFungibleTokenAttributes{}, tokens.PayloadTypeTransferFungibleToken, existingTokenID),
			attr: &tokens.TransferFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "invalid counter",
		},
		{
			name: "empty token type id",
			tx:   createTransactionOrder(t, &tokens.TransferFungibleTokenAttributes{}, tokens.PayloadTypeTransferFungibleToken, existingTokenID),
			attr: &tokens.TransferFungibleTokenAttributes{
				TypeID:                       nil,
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token type id",
			tx:   createTransactionOrder(t, &tokens.TransferFungibleTokenAttributes{}, tokens.PayloadTypeTransferFungibleToken, existingTokenID),
			attr: &tokens.TransferFungibleTokenAttributes{
				TypeID:                       existingTokenTypeID2,
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			},
			wantErrStr: "invalid type identifier",
		},
		//{ // 'Always True' ignores the signature bytes
		//	name: "invalid token invariant predicate argument",
		//	tx:   createTransactionOrder(t, &tokens.TransferFungibleTokenAttributes{}, tokens.PayloadTypeTransferFungibleToken, existingTokenUnitID),
		//	attr: &tokens.TransferFungibleTokenAttributes{
		//		TypeID:                       existingTokenTypeUnitID,
		//		NewBearer:                    templates.AlwaysTrueBytes(),
		//		Value:                        existingTokenValue,
		//		Nonce:                        test.RandomBytes(32),
		//		Counter:                     make([]byte, 32),
		//		InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
		//	},
		//	wantErrStr: "invalid predicate",
		//},
	}

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateTransferFT(tt.tx, tt.attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestTransferFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	transferAttributes := &tokens.TransferFungibleTokenAttributes{
		TypeID:                       existingTokenTypeID,
		NewBearer:                    templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
		Value:                        existingTokenValue,
		Nonce:                        test.RandomBytes(32),
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{nil},
	}
	uID := existingTokenID
	tx := createTransactionOrder(t, transferAttributes, tokens.PayloadTypeTransferFungibleToken, uID)
	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	var roundNo uint64 = 10
	require.NoError(t, m.validateTransferFT(tx, transferAttributes, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo))))
	sm, err := m.executeTransferFT(tx, transferAttributes, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)

	require.Equal(t, transferAttributes.NewBearer, []byte(u.Bearer()))
	require.Equal(t, transferAttributes.Value, d.Value)
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, roundNo, d.T)
}

func TestSplitFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.SplitFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, nil),
			attr:       &tokens.SplitFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingTokenTypeID),
			attr:       &tokens.SplitFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, tokens.NewFungibleTokenID(nil, []byte{42})),
			attr:       &tokens.SplitFungibleTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token locked",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingLockedTokenID),
			attr: &tokens.SplitFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "token is locked",
		},
		{
			name: "invalid target value - exceeds the max value",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingTokenID),
			attr: &tokens.SplitFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue + 1,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: fmt.Sprintf("invalid token value: max allowed %v, got %v", existingTokenValue, existingTokenValue+1),
		},
		{
			name: "invalid value: target + remainder < original value",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingTokenID),
			attr: &tokens.SplitFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue - 2,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: `remaining value must equal to the original value minus target value`,
		},
		{
			name: "invalid value - remaining value is zero",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingTokenID),
			attr: &tokens.SplitFungibleTokenAttributes{
				TypeID:                       existingTokenTypeID,
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue,
				RemainingValue:               0,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: `when splitting a token the remaining value of the token must be greater than zero`,
		},
		{
			name: "invalid value - target value is zero",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingTokenID),
			attr: &tokens.SplitFungibleTokenAttributes{
				TypeID:                       existingTokenTypeID,
				NewBearer:                    templates.AlwaysTrueBytes(),
				RemainingValue:               existingTokenValue,
				TargetValue:                  0,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: `when splitting a token the value assigned to the new token must be greater than zero`,
		},
		{
			name: "invalid counter",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingTokenID),
			attr: &tokens.SplitFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue - 1,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "invalid counter",
		},
		{
			name: "empty token type id",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingTokenID),
			attr: &tokens.SplitFungibleTokenAttributes{
				TypeID:                       nil,
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue - 1,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token type id",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingTokenID),
			attr: &tokens.SplitFungibleTokenAttributes{
				TypeID:                       existingTokenTypeID2,
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue - 1,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			},
			wantErrStr: "invalid type identifier",
		},
		//{ // 'Always True' ignores the signature bytes
		//	name: "invalid token invariant predicate argument",
		//	tx:   createTransactionOrder(t, nil, tokens.PayloadTypeSplitFungibleToken, existingTokenUnitID),
		//	attr: &tokens.SplitFungibleTokenAttributes{
		//		TypeID:                       existingTokenTypeUnitID,
		//		NewBearer:                    templates.AlwaysTrueBytes(),
		//		TargetValue:                  existingTokenValue - 1,
		//		RemainingValue:               1,
		//		Nonce:                        test.RandomBytes(32),
		//		Counter:                     make([]byte, 32),
		//		InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
		//	},
		//	wantErrStr: "invalid predicate",
		//},
	}

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateSplitFT(tt.tx, tt.attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestSplitFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)

	var remainingBillValue uint64 = 10
	attr := &tokens.SplitFungibleTokenAttributes{
		TypeID:                       existingTokenTypeID,
		NewBearer:                    templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
		TargetValue:                  existingTokenValue - remainingBillValue,
		RemainingValue:               remainingBillValue,
		Nonce:                        test.RandomBytes(32),
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{nil},
	}
	uID := existingTokenID
	tx := createTransactionOrder(t, attr, tokens.PayloadTypeSplitFungibleToken, uID)
	var roundNo uint64 = 10
	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	require.NoError(t, m.validateSplitFT(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo))))
	sm, err := m.executeSplitFT(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)

	require.EqualValues(t, templates.AlwaysTrueBytes(), []byte(u.Bearer()))
	require.Equal(t, remainingBillValue, d.Value)
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, roundNo, d.T)

	newUnitID := tokens.NewFungibleTokenID(uID, tx.HashForNewUnitID(opts.hashAlgorithm))
	newUnit, err := opts.state.GetUnit(newUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, newUnit)
	require.IsType(t, &tokens.FungibleTokenData{}, newUnit.Data())

	newUnitData := newUnit.Data().(*tokens.FungibleTokenData)

	require.Equal(t, attr.NewBearer, []byte(newUnit.Bearer()))
	require.Equal(t, existingTokenValue-remainingBillValue, newUnitData.Value)
	require.Equal(t, uint64(0), newUnitData.Counter)
	require.Equal(t, roundNo, newUnitData.T)
}

func TestBurnFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.BurnFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeBurnFungibleToken, nil),
			attr:       &tokens.BurnFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeBurnFungibleToken, existingTokenTypeID),
			attr:       &tokens.BurnFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeBurnFungibleToken, tokens.NewFungibleTokenID(nil, []byte{42})),
			attr:       &tokens.BurnFungibleTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token locked",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeBurnFungibleToken, existingLockedTokenID),
			attr: &tokens.BurnFungibleTokenAttributes{
				TypeID:                       existingTokenTypeID,
				Value:                        existingTokenValue,
				TargetTokenCounter:           0,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "token is locked",
		},
		{
			name: "invalid value",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeBurnFungibleToken, existingTokenID),
			attr: &tokens.BurnFungibleTokenAttributes{
				TypeID:                       existingTokenTypeID,
				Value:                        existingTokenValue - 1,
				TargetTokenCounter:           0,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid counter",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeBurnFungibleToken, existingTokenID),
			attr: &tokens.BurnFungibleTokenAttributes{
				TypeID:                       existingTokenTypeID,
				Value:                        existingTokenValue,
				TargetTokenCounter:           0,
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "invalid counter",
		},
		//{ // 'Always True' ignores the signature bytes
		//	name: "invalid token invariant predicate argument",
		//	tx:   createTransactionOrder(t, nil, tokens.PayloadTypeBurnFungibleToken, existingTokenUnitID),
		//	attr: &tokens.BurnFungibleTokenAttributes{
		//		TypeID:                       existingTokenTypeUnitID,
		//		Value:                        existingTokenValue,
		//		TargetTokenCounter:          test.RandomBytes(32),
		//		Counter:                     make([]byte, 32),
		//		InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
		//	},
		//	wantErrStr: "invalid predicate",
		//},
		{
			name: "invalid token type",
			tx:   createTransactionOrder(t, nil, tokens.PayloadTypeBurnFungibleToken, existingTokenID),
			attr: &tokens.BurnFungibleTokenAttributes{
				TypeID: func() []byte {
					r := tokens.NewFungibleTokenTypeID(nil, []byte{42})
					return r[:]
				}(),
				Value:                        existingTokenValue,
				TargetTokenCounter:           0,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			},
			wantErrStr: "type of token to burn does not matches the actual type of the token",
		},
	}

	m, err := NewFungibleTokensModule(defaultOpts(t))
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateBurnFT(tt.tx, tt.attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestBurnFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	burnAttributes := &tokens.BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeID,
		Value:                        existingTokenValue,
		TargetTokenCounter:           0,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{nil},
	}
	uID := existingTokenID
	tx := createTransactionOrder(t, burnAttributes, tokens.PayloadTypeBurnFungibleToken, uID)
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
	require.Equal(t, templates.AlwaysFalseBytes(), u.Bearer())

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
		TypeID:                       existingTokenTypeID,
		Value:                        existingTokenValue,
		TargetTokenID:                existingLockedTokenID,
		TargetTokenCounter:           0,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	burnTx := createTxRecord(t, existingTokenID, burnAttributes, tokens.PayloadTypeBurnFungibleToken)
	roundNo := uint64(10)
	sm, err := txExecutors.ValidateAndExecute(burnTx.TransactionOrder, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)

	burnTxProof := testblock.CreateProof(t, burnTx, signer, testblock.WithSystemIdentifier(tokens.DefaultSystemID))
	joinAttr := &tokens.JoinFungibleTokenAttributes{
		BurnTransactions:             []*types.TransactionRecord{burnTx},
		Proofs:                       []*types.TxProof{burnTxProof},
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	joinTx := createTx(t, existingLockedTokenID, joinAttr, tokens.PayloadTypeJoinFungibleToken)

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

	burnTxInvalidTargetTokenID := createTxRecord(t, existingTokenID, &tokens.BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeID,
		Value:                        existingTokenValue,
		TargetTokenID:                test.RandomBytes(32),
		TargetTokenCounter:           0,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{nil},
	}, tokens.PayloadTypeBurnFungibleToken)
	burnTxInvalidTargetTokenCounter := createTxRecord(t, existingTokenID, &tokens.BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeID,
		Value:                        existingTokenValue,
		TargetTokenID:                existingTokenID,
		TargetTokenCounter:           0,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{nil},
	}, tokens.PayloadTypeBurnFungibleToken)
	burnTx1 := createTxRecord(t, existingTokenID, &tokens.BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeID,
		Value:                        existingTokenValue,
		TargetTokenID:                existingTokenID,
		TargetTokenCounter:           0,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{nil},
	}, tokens.PayloadTypeBurnFungibleToken)
	burnTx2 := createTxRecord(t, existingTokenID2, &tokens.BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeID2,
		Value:                        existingTokenValue,
		TargetTokenID:                existingTokenID2,
		TargetTokenCounter:           0,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{nil},
	}, tokens.PayloadTypeBurnFungibleToken)
	maxUintValueTokenID := tokens.NewFungibleTokenID(nil, []byte{1, 0, 0, 2})
	burnTx3 := createTxRecord(t, maxUintValueTokenID, &tokens.BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeID2,
		Value:                        math.MaxUint64,
		TargetTokenID:                maxUintValueTokenID,
		TargetTokenCounter:           0,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{nil},
	}, tokens.PayloadTypeBurnFungibleToken)
	proofInvalidTargetTokenID := testblock.CreateProof(t, burnTxInvalidTargetTokenID, signer)
	proofInvalidTargetTokenCounter := testblock.CreateProof(t, burnTxInvalidTargetTokenCounter, signer)
	proofBurnTx2 := testblock.CreateProof(t, burnTx2, signer)
	proofBurnTx3 := testblock.CreateProof(t, burnTx3, signer)

	// create block with 3 burn txs
	var burnTxs []*types.TransactionRecord
	for i := uint8(3); i >= 1; i-- {
		burnTx := createTxRecord(t, tokens.NewFungibleTokenID(nil, []byte{i}), &tokens.BurnFungibleTokenAttributes{
			TypeID:                       existingTokenTypeID,
			Value:                        existingTokenValue,
			TargetTokenID:                existingTokenID,
			TargetTokenCounter:           0,
			Counter:                      0,
			InvariantPredicateSignatures: [][]byte{nil},
		}, tokens.PayloadTypeBurnFungibleToken)
		burnTxs = append(burnTxs, burnTx)
	}
	proofs := testblock.CreateProofs(t, burnTxs, signer, testblock.WithSystemIdentifier(tokens.DefaultSystemID))

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.JoinFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, &tokens.JoinFungibleTokenAttributes{}, tokens.PayloadTypeJoinFungibleToken, nil),
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, &tokens.JoinFungibleTokenAttributes{}, tokens.PayloadTypeJoinFungibleToken, existingTokenTypeID),
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, &tokens.JoinFungibleTokenAttributes{}, tokens.PayloadTypeJoinFungibleToken, tokens.NewFungibleTokenID(nil, []byte{42})),
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "invalid counter",
			tx: createTx(t, existingTokenID, &tokens.JoinFungibleTokenAttributes{
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{nil},
			}, tokens.PayloadTypeJoinFungibleToken),
			wantErrStr: "invalid counter",
		},
		{
			name: "token identifiers in wrong order",
			tx: createTx(t, existingTokenID, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions:             burnTxs,
				Proofs:                       proofs,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, tokens.PayloadTypeJoinFungibleToken),
			wantErrStr: "burn tx orders are not listed in strictly increasing order of token identifiers",
		},
		{
			name: "source not burned - invalid target token id",
			tx: createTx(t, existingTokenID, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTxInvalidTargetTokenID},
				Proofs:                       []*types.TxProof{proofInvalidTargetTokenID},
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, tokens.PayloadTypeJoinFungibleToken),
			wantErrStr: "burn tx target token id does not match with join transaction unit id",
		},
		{
			name: "source not burned - invalid target token counter",
			tx: createTx(t, existingTokenID, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTxInvalidTargetTokenCounter},
				Proofs:                       []*types.TxProof{proofInvalidTargetTokenCounter},
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, tokens.PayloadTypeJoinFungibleToken),
			wantErrStr: "burn tx target token counter does not match with join transaction counter",
		},
		{
			name: "invalid source token type",
			tx: createTx(t, existingTokenID, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTx2},
				Proofs:                       []*types.TxProof{proofInvalidTargetTokenID},
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, tokens.PayloadTypeJoinFungibleToken),
			wantErrStr: "the type of the burned source token does not match the type of target token",
		},
		{
			name: "proof is not valid",
			tx: createTx(t, existingTokenID, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTx1},
				Proofs:                       []*types.TxProof{proofBurnTx2},
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, tokens.PayloadTypeBurnFungibleToken),
			wantErrStr: "proof is not valid",
		},
		{
			name: "uint64 overflow",
			tx: createTx(t, existingTokenID2, &tokens.JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTx3},
				Proofs:                       []*types.TxProof{proofBurnTx3},
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{nil},
			}, tokens.PayloadTypeBurnFungibleToken),
			wantErrStr: "invalid sum of tokens: uint64 overflow",
		},
	}

	m, err := NewFungibleTokensModule(opts)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.JoinFungibleTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			err := m.validateJoinFT(tt.tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
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
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingTokenTypeID2, templates.AlwaysTrueBytes(), &tokens.FungibleTokenTypeData{
		Symbol:                   "ALPHA2",
		Name:                     "A long name for ALPHA2",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		ParentTypeID:             nil,
		DecimalPlaces:            5,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.AlwaysTrueBytes(),
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

func createTx(t *testing.T, unitID types.UnitID, attributes any, payloadType string) *types.TransactionOrder {
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithAttributes(attributes),
		testtransaction.WithPayloadType(payloadType),
	)
}

func createTxRecord(t *testing.T, unitID types.UnitID, attributes any, payloadType string) *types.TransactionRecord {
	return testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithAttributes(attributes),
		testtransaction.WithPayloadType(payloadType),
	)
}

func createTransactionOrder(t *testing.T, attr any, payloadType string, unitID types.UnitID) *types.TransactionOrder {
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithPayloadType(payloadType),
		testtransaction.WithAttributes(attr),
		testtransaction.WithSystemID(tokens.DefaultSystemID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
	)
}
