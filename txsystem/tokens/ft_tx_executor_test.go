package tokens

import (
	gocrypto "crypto"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	abcrypto "github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/predicates/templates"
	"github.com/alphabill-org/alphabill/validator/internal/state"
	test "github.com/alphabill-org/alphabill/validator/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/validator/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/validator/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/validator/internal/testutils/transaction"
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
	existingTokenTypeUnitID   = NewFungibleTokenTypeID(nil, []byte{1})
	existingTokenTypeUnitID2  = NewFungibleTokenTypeID(nil, []byte{1, 0, 0, 1})
	existingTokenUnitID       = NewFungibleTokenID(nil, []byte{0x02})
	existingTokenUnitID2      = NewFungibleTokenID(nil, []byte{0xaa})
	existingLockedTokenUnitID = NewFungibleTokenID(nil, []byte{0xbb})
	validUnitID               = NewFungibleTokenID(nil, []byte{100})
)

func TestCreateFungibleTokenType_NotOk(t *testing.T) {
	unitID, err := NewRandomFungibleTokenTypeID(nil)
	require.NoError(t, err)

	validTxOrder := &types.TransactionOrder{
		Payload: &types.Payload{
			Type:   PayloadTypeCreateFungibleTokenType,
			UnitID: unitID,
		},
	}
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *CreateFungibleTokenTypeAttributes
		options    *Options
		wantErrStr string
	}{
		{
			name: "nil unitID",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeCreateFungibleTokenType,
					UnitID: nil,
				},
			},
			attr:       &CreateFungibleTokenTypeAttributes{},
			options:    defaultOpts(t),
			wantErrStr: "invalid unit ID",
		},
		{
			name: "unitID has wrong type",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeCreateFungibleTokenType,
					UnitID: existingTokenUnitID,
				},
			},
			attr:       &CreateFungibleTokenTypeAttributes{},
			options:    defaultOpts(t),
			wantErrStr: "invalid unit ID",
		},
		{
			name: "parentTypeID has wrong type",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeCreateFungibleTokenType,
					UnitID: existingTokenTypeUnitID,
				},
			},
			attr:       &CreateFungibleTokenTypeAttributes{ParentTypeID: existingTokenUnitID},
			options:    defaultOpts(t),
			wantErrStr: "invalid parent type ID",
		},
		{
			name:       "symbol length exceeds the allowed maximum",
			tx:         validTxOrder,
			attr:       &CreateFungibleTokenTypeAttributes{Symbol: invalidSymbol},
			options:    defaultOpts(t),
			wantErrStr: "symbol length exceeds the allowed maximum of 16 bytes",
		},
		{
			name:       "name length exceeds the allowed maximum",
			tx:         validTxOrder,
			attr:       &CreateFungibleTokenTypeAttributes{Name: invalidName},
			options:    defaultOpts(t),
			wantErrStr: "name length exceeds the allowed maximum of 256 bytes",
		},
		{
			name: "icon type length exceeds the allowed maximum",
			tx:   validTxOrder,
			attr: &CreateFungibleTokenTypeAttributes{
				Symbol: validSymbol,
				Icon:   &Icon{Type: invalidIconType, Data: test.RandomBytes(maxIconDataLength)},
			},
			options:    defaultOpts(t),
			wantErrStr: "icon type length exceeds the allowed maximum of 64 bytes",
		},
		{
			name: "icon data length exceeds the allowed maximum",

			tx: validTxOrder,
			attr: &CreateFungibleTokenTypeAttributes{
				Symbol: validSymbol,
				Icon:   &Icon{Type: validIconType, Data: test.RandomBytes(maxIconDataLength + 1)},
			},
			options:    defaultOpts(t),
			wantErrStr: "icon data length exceeds the allowed maximum of 64 KiB",
		},
		{
			name:       "decimal places > 8",
			tx:         validTxOrder,
			attr:       &CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 9},
			options:    defaultOpts(t),
			wantErrStr: "invalid decimal places. maximum allowed value 8, got 9",
		},
		{
			name: "unit with given ID exists",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeCreateFungibleTokenType,
					UnitID: existingTokenTypeUnitID,
				},
			},
			attr:       &CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 5},
			options:    defaultOpts(t),
			wantErrStr: fmt.Sprintf("unit %s exists", existingTokenTypeUnitID),
		},
		{
			name:       "parent.decimals != tx.attributes.decimalPlaces",
			tx:         validTxOrder,
			attr:       &CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 6, ParentTypeID: existingTokenTypeUnitID},
			options:    defaultOpts(t),
			wantErrStr: "invalid decimal places. allowed 5, got 6",
		},
		{
			name:       "parent does not exist",
			tx:         validTxOrder,
			attr:       &CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 6, ParentTypeID: unitID},
			options:    defaultOpts(t),
			wantErrStr: fmt.Sprintf("item %s does not exist", unitID),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := handleCreateFungibleTokenTypeTx(tt.options)(tt.tx, tt.attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestCreateFungibleTokenType_CreateSingleType_Ok(t *testing.T) {
	opts := defaultOpts(t)
	attributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbol,
		Name:                     validName,
		Icon:                     &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeID:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.AlwaysFalseBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
	}

	unitID := NewFungibleTokenTypeID(nil, []byte{7})
	sm, err := handleCreateFungibleTokenTypeTx(opts)(
		createTransactionOrder(t, attributes, PayloadTypeCreateFungibleTokenType, unitID),
		attributes,
		10,
	)
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := opts.state.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	require.IsType(t, &fungibleTokenTypeData{}, u.Data())
	d := u.Data().(*fungibleTokenTypeData)
	require.Equal(t, attributes.Symbol, d.symbol)
	require.Equal(t, attributes.Name, d.name)
	require.Equal(t, attributes.Icon.Type, d.icon.Type)
	require.Equal(t, attributes.Icon.Data, d.icon.Data)
	require.Equal(t, attributes.DecimalPlaces, d.decimalPlaces)
	require.Equal(t, attributes.SubTypeCreationPredicate, d.subTypeCreationPredicate)
	require.Equal(t, attributes.TokenCreationPredicate, d.tokenCreationPredicate)
	require.Equal(t, attributes.InvariantPredicate, d.invariantPredicate)
	require.Nil(t, d.parentTypeId)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_Ok(t *testing.T) {
	opts := defaultOpts(t)

	parentAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbol,
		Name:                     validName,
		Icon:                     &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeID:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysFalseBytes(),
		InvariantPredicate:       templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
	}
	parentID := NewFungibleTokenTypeID(nil, []byte{19})
	parentTx := createTransactionOrder(t, parentAttributes, PayloadTypeCreateFungibleTokenType, parentID)

	childID := NewFungibleTokenTypeID(nil, []byte{20})
	childAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                             validSymbol + "_CHILD",
		Name:                               validName + "_CHILD",
		Icon:                               &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeID:                       parentID,
		DecimalPlaces:                      6,
		SubTypeCreationPredicate:           templates.AlwaysFalseBytes(),
		TokenCreationPredicate:             templates.AlwaysTrueBytes(),
		InvariantPredicate:                 templates.AlwaysTrueBytes(),
		SubTypeCreationPredicateSignatures: [][]byte{nil},
	}

	childTx := createTransactionOrder(t, childAttributes, PayloadTypeCreateFungibleTokenType, childID)

	sm, err := handleCreateFungibleTokenTypeTx(opts)(parentTx, parentAttributes, 10)
	require.NoError(t, err)
	require.NotNil(t, sm)
	sm, err = handleCreateFungibleTokenTypeTx(opts)(childTx, childAttributes, 11)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(childID, false)
	require.NoError(t, err)
	require.NotNil(t, u)

	require.IsType(t, &fungibleTokenTypeData{}, u.Data())
	d := u.Data().(*fungibleTokenTypeData)
	require.Equal(t, childAttributes.Symbol, d.symbol)
	require.Equal(t, childAttributes.Name, d.name)
	require.Equal(t, childAttributes.Icon.Type, d.icon.Type)
	require.Equal(t, childAttributes.Icon.Data, d.icon.Data)
	require.Equal(t, childAttributes.DecimalPlaces, d.decimalPlaces)
	require.Equal(t, childAttributes.SubTypeCreationPredicate, d.subTypeCreationPredicate)
	require.Equal(t, childAttributes.TokenCreationPredicate, d.tokenCreationPredicate)
	require.Equal(t, childAttributes.InvariantPredicate, d.invariantPredicate)
	require.Equal(t, types.UnitID(parentID), d.parentTypeId)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_InvalidCreationPredicateSignature(t *testing.T) {
	opts := defaultOpts(t)

	parentAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbol,
		ParentTypeID:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
		TokenCreationPredicate:   templates.AlwaysFalseBytes(),
		InvariantPredicate:       templates.NewP2pkh256BytesFromKeyHash(make([]byte, 32)),
	}
	parentID := NewFungibleTokenTypeID(nil, []byte{19})
	parentTx := createTransactionOrder(t, parentAttributes, PayloadTypeCreateFungibleTokenType, parentID)

	childID := NewFungibleTokenTypeID(nil, []byte{20})
	childAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                             validSymbol + "_CHILD",
		ParentTypeID:                       parentID,
		DecimalPlaces:                      6,
		SubTypeCreationPredicate:           templates.AlwaysFalseBytes(),
		TokenCreationPredicate:             templates.AlwaysTrueBytes(),
		InvariantPredicate:                 templates.AlwaysTrueBytes(),
		SubTypeCreationPredicateSignatures: [][]byte{[]byte("invalid")},
	}
	childTx := createTransactionOrder(t, childAttributes, PayloadTypeCreateFungibleTokenType, childID)

	sm, err := handleCreateFungibleTokenTypeTx(opts)(parentTx, parentAttributes, 10)
	require.NoError(t, err)
	require.NotNil(t, sm)

	sm, err = handleCreateFungibleTokenTypeTx(opts)(childTx, childAttributes, 11)
	require.ErrorContains(t, err, "invalid predicate")
	require.Nil(t, sm)
}

func TestMintFungibleToken_NotOk(t *testing.T) {
	missingTypeID := NewFungibleTokenTypeID(nil, []byte{99})
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *MintFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name: "unit ID is nil",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeMintFungibleToken,
					UnitID: nil,
				},
			},
			attr:       &MintFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name: "unit ID has wrong type",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeMintFungibleToken,
					UnitID: existingTokenTypeUnitID,
				},
			},
			attr:       &MintFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name: "type ID has wrong type",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeMintFungibleToken,
					UnitID: existingTokenTypeUnitID,
				},
			},
			attr:       &MintFungibleTokenAttributes{TypeID: existingTokenUnitID},
			wantErrStr: "invalid unit ID",
		},
		{
			name: "unit with given ID exists",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeMintFungibleToken,
					UnitID: existingTokenUnitID,
				},
			},
			attr:       &MintFungibleTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit with id %s already exists", existingTokenUnitID),
		},
		{
			name: "token type does not exist",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeMintFungibleToken,
					UnitID: validUnitID,
				},
			},
			attr: &MintFungibleTokenAttributes{
				Bearer:                           templates.AlwaysTrueBytes(),
				TypeID:                           missingTypeID,
				Value:                            1000,
				TokenCreationPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: fmt.Sprintf("item %s does not exist", missingTypeID),
		},
		//{ // 'Always True' ignores the signature bytes
		//	name: "invalid token creation predicate argument",
		//	tx: &types.TransactionOrder{
		//		Payload: &types.Payload{
		//			Type:   PayloadTypeMintFungibleToken,
		//			UnitID: validUnitID,
		//		},
		//	},
		//	attr: &MintFungibleTokenAttributes{
		//		Bearer:                           templates.AlwaysTrueBytes(),
		//		TypeID:                           existingTokenTypeUnitID,
		//		Value:                            1000,
		//		TokenCreationPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
		//	},
		//	wantErrStr: "invalid predicate",
		//},
		{
			name: "invalid value - zero",
			tx: &types.TransactionOrder{
				Payload: &types.Payload{
					Type:   PayloadTypeMintFungibleToken,
					UnitID: validUnitID,
				},
			},
			attr: &MintFungibleTokenAttributes{
				Bearer:                           templates.AlwaysTrueBytes(),
				TypeID:                           existingTokenTypeUnitID,
				Value:                            0,
				TokenCreationPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: `token must have value greater than zero`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := handleMintFungibleTokenTx(defaultOpts(t))(tt.tx, tt.attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestMintFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	attributes := &MintFungibleTokenAttributes{
		Bearer:                           templates.AlwaysTrueBytes(),
		TypeID:                           existingTokenTypeUnitID,
		Value:                            1000,
		TokenCreationPredicateSignatures: [][]byte{nil},
	}
	tokenID := validUnitID
	tx := createTransactionOrder(t, attributes, PayloadTypeMintFungibleToken, tokenID)

	sm, err := handleMintFungibleTokenTx(opts)(tx, attributes, 10)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(tokenID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data())

	d := u.Data().(*fungibleTokenData)
	require.Equal(t, types.UnitID(attributes.TypeID), d.tokenType)
	require.Equal(t, attributes.Value, d.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, uint64(10), d.t)
	require.Equal(t, attributes.Bearer, []byte(u.Bearer()))
}

func TestTransferFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *TransferFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, PayloadTypeTransferFungibleToken, nil),
			attr:       &TransferFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, PayloadTypeTransferFungibleToken, existingTokenTypeUnitID),
			attr:       &TransferFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, PayloadTypeTransferFungibleToken, NewFungibleTokenID(nil, []byte{42})),
			attr:       &TransferFungibleTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token locked",
			tx:   createTransactionOrder(t, &TransferFungibleTokenAttributes{}, PayloadTypeTransferFungibleToken, existingLockedTokenUnitID),
			attr: &TransferFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "token is locked",
		},
		{
			name: "invalid value",
			tx:   createTransactionOrder(t, &TransferFungibleTokenAttributes{}, PayloadTypeTransferFungibleToken, existingTokenUnitID),
			attr: &TransferFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue - 1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid backlink",
			tx:   createTransactionOrder(t, &TransferFungibleTokenAttributes{}, PayloadTypeTransferFungibleToken, existingTokenUnitID),
			attr: &TransferFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "invalid backlink",
		},
		{
			name: "empty token type id",
			tx:   createTransactionOrder(t, &TransferFungibleTokenAttributes{}, PayloadTypeTransferFungibleToken, existingTokenUnitID),
			attr: &TransferFungibleTokenAttributes{
				TypeID:                       nil,
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token type id",
			tx:   createTransactionOrder(t, &TransferFungibleTokenAttributes{}, PayloadTypeTransferFungibleToken, existingTokenUnitID),
			attr: &TransferFungibleTokenAttributes{
				TypeID:                       existingTokenTypeUnitID2,
				NewBearer:                    templates.AlwaysTrueBytes(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			},
			wantErrStr: "invalid type identifier",
		},
		//{ // 'Always True' ignores the signature bytes
		//	name: "invalid token invariant predicate argument",
		//	tx:   createTransactionOrder(t, &TransferFungibleTokenAttributes{}, PayloadTypeTransferFungibleToken, existingTokenUnitID),
		//	attr: &TransferFungibleTokenAttributes{
		//		TypeID:                       existingTokenTypeUnitID,
		//		NewBearer:                    templates.AlwaysTrueBytes(),
		//		Value:                        existingTokenValue,
		//		Nonce:                        test.RandomBytes(32),
		//		Backlink:                     make([]byte, 32),
		//		InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
		//	},
		//	wantErrStr: "invalid predicate",
		//},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := handleTransferFungibleTokenTx(defaultOpts(t))(tt.tx, tt.attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestTransferFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	transferAttributes := &TransferFungibleTokenAttributes{
		TypeID:                       existingTokenTypeUnitID,
		NewBearer:                    templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
		Value:                        existingTokenValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{nil},
	}
	uID := existingTokenUnitID
	tx := createTransactionOrder(t, transferAttributes, PayloadTypeTransferFungibleToken, uID)

	var roundNr uint64 = 10
	sm, err := handleTransferFungibleTokenTx(opts)(tx, transferAttributes, roundNr)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data())
	d := u.Data().(*fungibleTokenData)

	require.Equal(t, transferAttributes.NewBearer, []byte(u.Bearer()))
	require.Equal(t, transferAttributes.Value, d.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, roundNr, d.t)
}

func TestSplitFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *SplitFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, nil),
			attr:       &SplitFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingTokenTypeUnitID),
			attr:       &SplitFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, NewFungibleTokenID(nil, []byte{42})),
			attr:       &SplitFungibleTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token locked",
			tx:   createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingLockedTokenUnitID),
			attr: &SplitFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "token is locked",
		},
		{
			name: "invalid target value - exceeds the max value",
			tx:   createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingTokenUnitID),
			attr: &SplitFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue + 1,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: fmt.Sprintf("invalid token value: max allowed %v, got %v", existingTokenValue, existingTokenValue+1),
		},
		{
			name: "invalid value: target + remainder < original value",
			tx:   createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingTokenUnitID),
			attr: &SplitFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue - 2,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: `remaining value must equal to the original value minus target value`,
		},
		{
			name: "invalid value - remaining value is zero",
			tx:   createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingTokenUnitID),
			attr: &SplitFungibleTokenAttributes{
				TypeID:                       existingTokenTypeUnitID,
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue,
				RemainingValue:               0,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: `when splitting a token the remaining value of the token must be greater than zero`,
		},
		{
			name: "invalid value - target value is zero",
			tx:   createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingTokenUnitID),
			attr: &SplitFungibleTokenAttributes{
				TypeID:                       existingTokenTypeUnitID,
				NewBearer:                    templates.AlwaysTrueBytes(),
				RemainingValue:               existingTokenValue,
				TargetValue:                  0,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: `when splitting a token the value assigned to the new token must be greater than zero`,
		},
		{
			name: "invalid backlink",
			tx:   createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingTokenUnitID),
			attr: &SplitFungibleTokenAttributes{
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue - 1,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "invalid backlink",
		},
		{
			name: "empty token type id",
			tx:   createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingTokenUnitID),
			attr: &SplitFungibleTokenAttributes{
				TypeID:                       nil,
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue - 1,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token type id",
			tx:   createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingTokenUnitID),
			attr: &SplitFungibleTokenAttributes{
				TypeID:                       existingTokenTypeUnitID2,
				NewBearer:                    templates.AlwaysTrueBytes(),
				TargetValue:                  existingTokenValue - 1,
				RemainingValue:               1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			},
			wantErrStr: "invalid type identifier",
		},
		//{ // 'Always True' ignores the signature bytes
		//	name: "invalid token invariant predicate argument",
		//	tx:   createTransactionOrder(t, nil, PayloadTypeSplitFungibleToken, existingTokenUnitID),
		//	attr: &SplitFungibleTokenAttributes{
		//		TypeID:                       existingTokenTypeUnitID,
		//		NewBearer:                    templates.AlwaysTrueBytes(),
		//		TargetValue:                  existingTokenValue - 1,
		//		RemainingValue:               1,
		//		Nonce:                        test.RandomBytes(32),
		//		Backlink:                     make([]byte, 32),
		//		InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
		//	},
		//	wantErrStr: "invalid predicate",
		//},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := handleSplitFungibleTokenTx(defaultOpts(t))(tt.tx, tt.attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestSplitFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)

	var remainingBillValue uint64 = 10
	attr := &SplitFungibleTokenAttributes{
		TypeID:                       existingTokenTypeUnitID,
		NewBearer:                    templates.NewP2pkh256BytesFromKeyHash(test.RandomBytes(32)),
		TargetValue:                  existingTokenValue - remainingBillValue,
		RemainingValue:               remainingBillValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{nil},
	}
	uID := existingTokenUnitID
	tx := createTransactionOrder(t, attr, PayloadTypeSplitFungibleToken, uID)
	var roundNr uint64 = 10
	sm, err := handleSplitFungibleTokenTx(opts)(tx, attr, roundNr)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(uID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data())
	d := u.Data().(*fungibleTokenData)

	require.EqualValues(t, templates.AlwaysTrueBytes(), []byte(u.Bearer()))
	require.Equal(t, remainingBillValue, d.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, roundNr, d.t)

	newUnitID := NewFungibleTokenID(uID, HashForIDCalculation(tx, opts.hashAlgorithm))
	newUnit, err := opts.state.GetUnit(newUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, newUnit)
	require.IsType(t, &fungibleTokenData{}, newUnit.Data())

	newUnitData := newUnit.Data().(*fungibleTokenData)

	require.Equal(t, attr.NewBearer, []byte(newUnit.Bearer()))
	require.Equal(t, existingTokenValue-remainingBillValue, newUnitData.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), newUnitData.backlink)
	require.Equal(t, roundNr, newUnitData.t)
}

func TestBurnFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *BurnFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, PayloadTypeBurnFungibleToken, nil),
			attr:       &BurnFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, PayloadTypeBurnFungibleToken, existingTokenTypeUnitID),
			attr:       &BurnFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, PayloadTypeBurnFungibleToken, NewFungibleTokenID(nil, []byte{42})),
			attr:       &BurnFungibleTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token locked",
			tx:   createTransactionOrder(t, nil, PayloadTypeBurnFungibleToken, existingLockedTokenUnitID),
			attr: &BurnFungibleTokenAttributes{
				TypeID:                       existingTokenTypeUnitID,
				Value:                        existingTokenValue,
				TargetTokenBacklink:          test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "token is locked",
		},
		{
			name: "invalid value",
			tx:   createTransactionOrder(t, nil, PayloadTypeBurnFungibleToken, existingTokenUnitID),
			attr: &BurnFungibleTokenAttributes{
				TypeID:                       existingTokenTypeUnitID,
				Value:                        existingTokenValue - 1,
				TargetTokenBacklink:          test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid backlink",
			tx:   createTransactionOrder(t, nil, PayloadTypeBurnFungibleToken, existingTokenUnitID),
			attr: &BurnFungibleTokenAttributes{
				TypeID:                       existingTokenTypeUnitID,
				Value:                        existingTokenValue,
				TargetTokenBacklink:          test.RandomBytes(32),
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{nil},
			},
			wantErrStr: "invalid backlink",
		},
		//{ // 'Always True' ignores the signature bytes
		//	name: "invalid token invariant predicate argument",
		//	tx:   createTransactionOrder(t, nil, PayloadTypeBurnFungibleToken, existingTokenUnitID),
		//	attr: &BurnFungibleTokenAttributes{
		//		TypeID:                       existingTokenTypeUnitID,
		//		Value:                        existingTokenValue,
		//		TargetTokenBacklink:          test.RandomBytes(32),
		//		Backlink:                     make([]byte, 32),
		//		InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
		//	},
		//	wantErrStr: "invalid predicate",
		//},
		{
			name: "invalid token type",
			tx:   createTransactionOrder(t, nil, PayloadTypeBurnFungibleToken, existingTokenUnitID),
			attr: &BurnFungibleTokenAttributes{
				TypeID: func() []byte {
					r := NewFungibleTokenTypeID(nil, []byte{42})
					return r[:]
				}(),
				Value:                        existingTokenValue,
				TargetTokenBacklink:          test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			},
			wantErrStr: "type of token to burn does not matches the actual type of the token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := handleBurnFungibleTokenTx(defaultOpts(t))(tt.tx, tt.attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestBurnFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	burnAttributes := &BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeUnitID,
		Value:                        existingTokenValue,
		TargetTokenBacklink:          test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{nil},
	}
	uID := existingTokenUnitID
	tx := createTransactionOrder(t, burnAttributes, PayloadTypeBurnFungibleToken, uID)
	var roundNr uint64 = 10
	sm, err := handleBurnFungibleTokenTx(opts)(tx, burnAttributes, roundNr)
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := opts.state.GetUnit(uID, false)
	require.Nil(t, u)
	require.ErrorContains(t, err, fmt.Sprintf("item %s does not exist", uID))
}

func TestJoinFungibleToken_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}

	burnAttributes := &BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeUnitID,
		Value:                        existingTokenValue,
		TargetTokenID:                existingLockedTokenUnitID,
		TargetTokenBacklink:          make([]byte, 32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
	}
	burnTx := createTxRecord(t, existingTokenUnitID, burnAttributes, PayloadTypeBurnFungibleToken)
	roundNumber := uint64(10)
	sm, err := handleBurnFungibleTokenTx(opts)(burnTx.TransactionOrder, burnAttributes, roundNumber)
	require.NoError(t, err)
	require.NotNil(t, sm)

	burnTxProof := testblock.CreateProof(t, burnTx, signer, testblock.WithSystemIdentifier(DefaultSystemIdentifier))
	joinAttr := &JoinFungibleTokenAttributes{
		BurnTransactions:             []*types.TransactionRecord{burnTx},
		Proofs:                       []*types.TxProof{burnTxProof},
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
	}
	joinTx := createTx(t, existingLockedTokenUnitID, burnAttributes, PayloadTypeBurnFungibleToken)
	sm, err = handleJoinFungibleTokenTx(opts)(joinTx, joinAttr, roundNumber)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify locked target unit was unlocked
	u, err := opts.state.GetUnit(existingLockedTokenUnitID, false)
	require.NoError(t, err)
	require.EqualValues(t, 0, u.Data().(*fungibleTokenData).locked)
}

func TestJoinFungibleToken_NotOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}

	burnTxInvalidTargetTokenID := createTxRecord(t, existingTokenUnitID, &BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeUnitID,
		Value:                        existingTokenValue,
		TargetTokenID:                test.RandomBytes(32),
		TargetTokenBacklink:          make([]byte, 32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{nil},
	}, PayloadTypeBurnFungibleToken)
	burnTxInvalidTargetTokenBacklink := createTxRecord(t, existingTokenUnitID, &BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeUnitID,
		Value:                        existingTokenValue,
		TargetTokenID:                existingTokenUnitID,
		TargetTokenBacklink:          test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{nil},
	}, PayloadTypeBurnFungibleToken)
	burnTx1 := createTxRecord(t, existingTokenUnitID, &BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeUnitID,
		Value:                        existingTokenValue,
		TargetTokenID:                existingTokenUnitID,
		TargetTokenBacklink:          make([]byte, 32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{nil},
	}, PayloadTypeBurnFungibleToken)
	burnTx2 := createTxRecord(t, existingTokenUnitID2, &BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeUnitID2,
		Value:                        existingTokenValue,
		TargetTokenID:                existingTokenUnitID2,
		TargetTokenBacklink:          test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{nil},
	}, PayloadTypeBurnFungibleToken)
	maxUintValueTokenID := NewFungibleTokenID(nil, []byte{1, 0, 0, 2})
	burnTx3 := createTxRecord(t, maxUintValueTokenID, &BurnFungibleTokenAttributes{
		TypeID:                       existingTokenTypeUnitID2,
		Value:                        math.MaxUint64,
		TargetTokenID:                maxUintValueTokenID,
		TargetTokenBacklink:          make([]byte, 32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{nil},
	}, PayloadTypeBurnFungibleToken)
	proofInvalidTargetTokenID := testblock.CreateProof(t, burnTxInvalidTargetTokenID, signer)
	proofInvalidTargetTokenBacklink := testblock.CreateProof(t, burnTxInvalidTargetTokenBacklink, signer)
	proofBurnTx2 := testblock.CreateProof(t, burnTx2, signer)
	proofBurnTx3 := testblock.CreateProof(t, burnTx3, signer)

	// create block with 3 burn txs
	var burnTxs []*types.TransactionRecord
	for i := uint8(3); i >= 1; i-- {
		burnTx := createTxRecord(t, NewFungibleTokenID(nil, []byte{i}), &BurnFungibleTokenAttributes{
			TypeID:                       existingTokenTypeUnitID,
			Value:                        existingTokenValue,
			TargetTokenID:                existingTokenUnitID,
			TargetTokenBacklink:          make([]byte, 32),
			Backlink:                     make([]byte, 32),
			InvariantPredicateSignatures: [][]byte{nil},
		}, PayloadTypeBurnFungibleToken)
		burnTxs = append(burnTxs, burnTx)
	}
	proofs := testblock.CreateProofs(t, burnTxs, signer, testblock.WithSystemIdentifier(DefaultSystemIdentifier))

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *JoinFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, &JoinFungibleTokenAttributes{}, PayloadTypeJoinFungibleToken, nil),
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, &JoinFungibleTokenAttributes{}, PayloadTypeJoinFungibleToken, existingTokenTypeUnitID),
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, &JoinFungibleTokenAttributes{}, PayloadTypeJoinFungibleToken, NewFungibleTokenID(nil, []byte{42})),
			wantErrStr: fmt.Sprintf("unit %s does not exist", NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "invalid backlink",
			tx: createTx(t, existingTokenUnitID, &JoinFungibleTokenAttributes{
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{nil},
			}, PayloadTypeJoinFungibleToken),
			wantErrStr: "invalid backlink",
		},
		{
			name: "token identifiers in wrong order",
			tx: createTx(t, existingTokenUnitID, &JoinFungibleTokenAttributes{
				BurnTransactions:             burnTxs,
				Proofs:                       proofs,
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeJoinFungibleToken),
			wantErrStr: "burn tx orders are not listed in strictly increasing order of token identifiers",
		},
		{
			name: "source not burned - invalid target token id",
			tx: createTx(t, existingTokenUnitID, &JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTxInvalidTargetTokenID},
				Proofs:                       []*types.TxProof{proofInvalidTargetTokenID},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeJoinFungibleToken),
			wantErrStr: "burn tx target token id does not match with join transaction unit id",
		},
		{
			name: "source not burned - invalid target token backlink",
			tx: createTx(t, existingTokenUnitID, &JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTxInvalidTargetTokenBacklink},
				Proofs:                       []*types.TxProof{proofInvalidTargetTokenBacklink},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeJoinFungibleToken),
			wantErrStr: "burn tx target token backlink does not match with join transaction backlink",
		},
		{
			name: "invalid source token type",
			tx: createTx(t, existingTokenUnitID, &JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTx2},
				Proofs:                       []*types.TxProof{proofInvalidTargetTokenID},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeJoinFungibleToken),
			wantErrStr: "the type of the burned source token does not match the type of target token",
		},
		{
			name: "proof is not valid",
			tx: createTx(t, existingTokenUnitID, &JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTx1},
				Proofs:                       []*types.TxProof{proofBurnTx2},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeBurnFungibleToken),
			wantErrStr: "proof is not valid",
		},
		{
			name: "uint64 overflow",
			tx: createTx(t, existingTokenUnitID2, &JoinFungibleTokenAttributes{
				BurnTransactions:             []*types.TransactionRecord{burnTx3},
				Proofs:                       []*types.TxProof{proofBurnTx3},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{nil},
			}, PayloadTypeBurnFungibleToken),
			wantErrStr: "invalid sum of tokens: uint64 overflow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &JoinFungibleTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := handleJoinFungibleTokenTx(opts)(tt.tx, attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func defaultOpts(t *testing.T) *Options {
	o := defaultOptions()
	o.state = initState(t)
	return o
}

func initState(t *testing.T) *state.State {
	s := state.NewEmptyState()
	err := s.Apply(state.AddUnit(existingTokenTypeUnitID, templates.AlwaysTrueBytes(), &fungibleTokenTypeData{
		symbol:                   "ALPHA",
		name:                     "A long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		parentTypeId:             nil,
		decimalPlaces:            5,
		subTypeCreationPredicate: templates.AlwaysTrueBytes(),
		tokenCreationPredicate:   templates.AlwaysTrueBytes(),
		invariantPredicate:       templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingTokenTypeUnitID2, templates.AlwaysTrueBytes(), &fungibleTokenTypeData{
		symbol:                   "ALPHA2",
		name:                     "A long name for ALPHA2",
		icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		parentTypeId:             nil,
		decimalPlaces:            5,
		subTypeCreationPredicate: templates.AlwaysTrueBytes(),
		tokenCreationPredicate:   templates.AlwaysTrueBytes(),
		invariantPredicate:       templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingTokenUnitID, templates.AlwaysTrueBytes(), &fungibleTokenData{
		tokenType: existingTokenTypeUnitID,
		value:     existingTokenValue,
		t:         0,
		backlink:  make([]byte, 32),
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingTokenUnitID2, templates.AlwaysTrueBytes(), &fungibleTokenData{
		tokenType: existingTokenTypeUnitID2,
		value:     existingTokenValue,
		t:         0,
		backlink:  make([]byte, 32),
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(existingLockedTokenUnitID, templates.AlwaysTrueBytes(), &fungibleTokenData{
		tokenType: existingTokenTypeUnitID,
		value:     existingTokenValue,
		t:         0,
		backlink:  make([]byte, 32),
		locked:    1,
	}))
	require.NoError(t, err)
	err = s.Apply(state.AddUnit(feeCreditID, templates.AlwaysTrueBytes(), &unit.FeeCreditRecord{
		Balance:  100,
		Backlink: make([]byte, 32),
		Timeout:  100,
	}))
	require.NoError(t, err)
	return s
}

func createTx(t *testing.T, unitID types.UnitID, attributes any, payloadType string) *types.TransactionOrder {
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithAttributes(attributes),
		testtransaction.WithPayloadType(payloadType),
	)
}

func createTxRecord(t *testing.T, unitID types.UnitID, attributes any, payloadType string) *types.TransactionRecord {
	return testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithAttributes(attributes),
		testtransaction.WithPayloadType(payloadType),
	)
}

func createTransactionOrder(t *testing.T, attr any, payloadType string, unitID types.UnitID) *types.TransactionOrder {
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithPayloadType(payloadType),
		testtransaction.WithAttributes(attr),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithFeeProof(nil),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           1000,
			MaxTransactionFee: 10,
			FeeCreditRecordID: feeCreditID,
		}),
	)
}
