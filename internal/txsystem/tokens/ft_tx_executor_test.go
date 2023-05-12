package tokens

import (
	gocrypto "crypto"
	"fmt"
	"math"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
)

const (
	invalidSymbol   = "♥ Alphabill ♥"
	invalidName     = "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥ We ♥ Alphabill♥♥"
	invalidIconType = "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥"
	validSymbol     = "BETA"
	validName       = "Long name for BETA"
	validIconType   = "image/png"
	validUnitID     = 0x0000000000000000000000000000000000000000000000000000000000000064

	existingTokenUnitID = 2
	existingTokenValue  = 1000

	existingTokenUnitID2 = 10000
)

var (
	existingTokenTypeUnitID      = uint256.NewInt(1)
	existingTokenTypeUnitIDBytes = util.Uint256ToBytes(existingTokenTypeUnitID)

	existingTokenTypeUnitID2      = uint256.NewInt(1001)
	existingTokenTypeUnitIDBytes2 = util.Uint256ToBytes(existingTokenTypeUnitID2)
)

func TestCreateFungibleTokenType_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *createFungibleTokenTypeWrapper
		options    *Options
		wantErrStr string
	}{
		{
			name: "unit ID is 0",
			tx: &createFungibleTokenTypeWrapper{
				wrapper:    createWrapper(t, uint256.NewInt(0)),
				attributes: &CreateFungibleTokenTypeAttributes{},
			},
			options:    defaultOpts(t),
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name: "symbol length exceeds the allowed maximum",
			tx: &createFungibleTokenTypeWrapper{
				wrapper:    createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &CreateFungibleTokenTypeAttributes{Symbol: invalidSymbol},
			},
			options:    defaultOpts(t),
			wantErrStr: "symbol length exceeds the allowed maximum of 16 bytes",
		},
		{
			name: "name length exceeds the allowed maximum",
			tx: &createFungibleTokenTypeWrapper{
				wrapper:    createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &CreateFungibleTokenTypeAttributes{Name: invalidName},
			},
			options:    defaultOpts(t),
			wantErrStr: "name length exceeds the allowed maximum of 256 bytes",
		},
		{
			name: "icon type length exceeds the allowed maximum",
			tx: &createFungibleTokenTypeWrapper{
				wrapper:    createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &CreateFungibleTokenTypeAttributes{
					Symbol: validSymbol,
					Icon: &Icon{Type: invalidIconType, Data: test.RandomBytes(maxIconDataLength)},
				},
			},
			options:    defaultOpts(t),
			wantErrStr: "icon type length exceeds the allowed maximum of 64 bytes",
		},
		{
			name: "icon data length exceeds the allowed maximum",
			tx: &createFungibleTokenTypeWrapper{
				wrapper:    createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &CreateFungibleTokenTypeAttributes{
					Symbol: validSymbol,
					Icon: &Icon{Type: validIconType, Data: test.RandomBytes(maxIconDataLength + 1)},
				},
			},
			options:    defaultOpts(t),
			wantErrStr: "icon data length exceeds the allowed maximum of 64 KiB",
		},
		{
			name: "decimal places > 8",
			tx: &createFungibleTokenTypeWrapper{
				wrapper:    createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 9},
			},
			options:    defaultOpts(t),
			wantErrStr: "invalid decimal places. maximum allowed value 8, got 9",
		},
		{
			name: "unit with given ID exists",
			tx: &createFungibleTokenTypeWrapper{
				wrapper:    createWrapper(t, existingTokenTypeUnitID),
				attributes: &CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 5},
			},
			options:    defaultOpts(t),
			wantErrStr: fmt.Sprintf("unit %v exists", existingTokenTypeUnitID),
		},
		{
			name: "parent.decimals != tx.attributes.decimalPlaces",
			tx: &createFungibleTokenTypeWrapper{
				wrapper:    createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 6, ParentTypeId: existingTokenTypeUnitIDBytes[:]},
			},
			options:    defaultOpts(t),
			wantErrStr: "invalid decimal places. allowed 5, got 6",
		},
		{
			name: "parent does not exist",
			tx: &createFungibleTokenTypeWrapper{
				wrapper:    createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &CreateFungibleTokenTypeAttributes{Symbol: validSymbol, DecimalPlaces: 6, ParentTypeId: util.Uint256ToBytes(uint256.NewInt(100))},
			},
			options:    defaultOpts(t),
			wantErrStr: fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(uint256.NewInt(validUnitID))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, handleCreateFungibleTokenTypeTx(tt.options)(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestCreateFungibleTokenType_CreateSingleType_Ok(t *testing.T) {
	opts := defaultOpts(t)
	attributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbol,
		Name:                     validName,
		Icon:                     &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeId:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: script.PredicateAlwaysFalse(),
		TokenCreationPredicate:   script.PredicateAlwaysTrue(),
		InvariantPredicate:       script.PredicatePayToPublicKeyHashDefault(make([]byte, 32)),
	}

	uID := uint256.NewInt(validUnitID)
	err := handleCreateFungibleTokenTypeTx(opts)(
		&createFungibleTokenTypeWrapper{
			wrapper:    createWrapper(t, uID),
			attributes: attributes,
		}, 10)
	require.NoError(t, err)

	u, err := opts.state.GetUnit(uID)
	require.NoError(t, err)
	require.NotNil(t, u)

	require.IsType(t, &fungibleTokenTypeData{}, u.Data)
	d := u.Data.(*fungibleTokenTypeData)
	require.Equal(t, attributes.Symbol, d.symbol)
	require.Equal(t, attributes.Name, d.name)
	require.Equal(t, attributes.Icon.GetType(), d.icon.GetType())
	require.Equal(t, attributes.Icon.GetData(), d.icon.GetData())
	require.Equal(t, attributes.DecimalPlaces, d.decimalPlaces)
	require.Equal(t, attributes.SubTypeCreationPredicate, d.subTypeCreationPredicate)
	require.Equal(t, attributes.TokenCreationPredicate, d.tokenCreationPredicate)
	require.Equal(t, attributes.InvariantPredicate, d.invariantPredicate)
	require.Equal(t, uint256.NewInt(0), d.parentTypeId)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_Ok(t *testing.T) {
	opts := defaultOpts(t)

	parentAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbol,
		Name:                     validName,
		Icon:                     &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeId:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
		TokenCreationPredicate:   script.PredicateAlwaysFalse(),
		InvariantPredicate:       script.PredicatePayToPublicKeyHashDefault(make([]byte, 32)),
	}
	parentID := uint256.NewInt(validUnitID)
	parentTx := &createFungibleTokenTypeWrapper{
		wrapper:    createWrapper(t, parentID),
		attributes: parentAttributes,
	}

	childID := uint256.NewInt(20)
	childAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                             validSymbol + "_CHILD",
		Name:                               validName + "_CHILD",
		Icon:                               &Icon{Type: validIconType, Data: []byte{1, 2, 3}},
		ParentTypeId:                       util.Uint256ToBytes(parentID),
		DecimalPlaces:                      6,
		SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
		TokenCreationPredicate:             script.PredicateAlwaysTrue(),
		InvariantPredicate:                 script.PredicateAlwaysTrue(),
		SubTypeCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
	childTx := &createFungibleTokenTypeWrapper{
		wrapper:    createWrapper(t, childID),
		attributes: childAttributes,
	}

	err := handleCreateFungibleTokenTypeTx(opts)(parentTx, 10)
	require.NoError(t, err)

	err = handleCreateFungibleTokenTypeTx(opts)(childTx, 11)
	require.NoError(t, err)

	u, err := opts.state.GetUnit(childID)
	require.NoError(t, err)
	require.NotNil(t, u)

	require.IsType(t, &fungibleTokenTypeData{}, u.Data)
	d := u.Data.(*fungibleTokenTypeData)
	require.Equal(t, childAttributes.Symbol, d.symbol)
	require.Equal(t, childAttributes.Name, d.name)
	require.Equal(t, childAttributes.Icon.GetType(), d.icon.GetType())
	require.Equal(t, childAttributes.Icon.GetData(), d.icon.GetData())
	require.Equal(t, childAttributes.DecimalPlaces, d.decimalPlaces)
	require.Equal(t, childAttributes.SubTypeCreationPredicate, d.subTypeCreationPredicate)
	require.Equal(t, childAttributes.TokenCreationPredicate, d.tokenCreationPredicate)
	require.Equal(t, childAttributes.InvariantPredicate, d.invariantPredicate)
	require.Equal(t, parentID, d.parentTypeId)
}

func TestCreateFungibleTokenType_CreateTokenTypeChain_InvalidCreationPredicateSignature(t *testing.T) {
	opts := defaultOpts(t)

	parentAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                   validSymbol,
		ParentTypeId:             nil,
		DecimalPlaces:            6,
		SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
		TokenCreationPredicate:   script.PredicateAlwaysFalse(),
		InvariantPredicate:       script.PredicatePayToPublicKeyHashDefault(make([]byte, 32)),
	}
	parentID := uint256.NewInt(validUnitID)
	parentTx := &createFungibleTokenTypeWrapper{
		wrapper:    createWrapper(t, parentID),
		attributes: parentAttributes,
	}

	parentIDBytes := parentID.Bytes32()
	childID := uint256.NewInt(20)
	childAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                             validSymbol + "_CHILD",
		ParentTypeId:                       parentIDBytes[:],
		DecimalPlaces:                      6,
		SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
		TokenCreationPredicate:             script.PredicateAlwaysTrue(),
		InvariantPredicate:                 script.PredicateAlwaysTrue(),
		SubTypeCreationPredicateSignatures: [][]byte{[]byte("invalid")},
	}
	childTx := &createFungibleTokenTypeWrapper{
		wrapper:    createWrapper(t, childID),
		attributes: childAttributes,
	}

	err := handleCreateFungibleTokenTypeTx(opts)(parentTx, 10)
	require.NoError(t, err)

	err = handleCreateFungibleTokenTypeTx(opts)(childTx, 11)
	require.ErrorContains(t, err, "invalid script format")
}

func TestMintFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *mintFungibleTokenWrapper
		wantErrStr string
	}{
		{
			name: "unit ID is 0",
			tx: &mintFungibleTokenWrapper{
				wrapper:    createWrapper(t, uint256.NewInt(0)),
				attributes: &MintFungibleTokenAttributes{},
			},
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name: "unit with given ID exists",
			tx: &mintFungibleTokenWrapper{
				wrapper:    createWrapper(t, existingTokenTypeUnitID),
				attributes: &MintFungibleTokenAttributes{},
			},
			wantErrStr: fmt.Sprintf("unit with id %v already exists", existingTokenTypeUnitID),
		},
		{
			name: "parent does not exist",
			tx: &mintFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &MintFungibleTokenAttributes{
					Bearer:                           script.PredicateAlwaysTrue(),
					Type:                             util.Uint256ToBytes(uint256.NewInt(100)),
					Value:                            1000,
					TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				},
			},

			wantErrStr: fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(uint256.NewInt(validUnitID))),
		},
		{
			name: "invalid token creation predicate argument",
			tx: &mintFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &MintFungibleTokenAttributes{
					Bearer:                           script.PredicateAlwaysTrue(),
					Type:                             existingTokenTypeUnitIDBytes[:],
					Value:                            1000,
					TokenCreationPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
				}},
			wantErrStr: "script execution result yielded false or non-clean stack",
		},
		{
			name: "invalid value - zero",
			tx: &mintFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(validUnitID)),
				attributes: &MintFungibleTokenAttributes{
					Bearer:                           script.PredicateAlwaysTrue(),
					Type:                             existingTokenTypeUnitIDBytes,
					Value:                            0,
					TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: `token must have value greater than zero`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, handleMintFungibleTokenTx(defaultOpts(t))(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestMintFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	attributes := &MintFungibleTokenAttributes{
		Bearer:                           script.PredicateAlwaysTrue(),
		Type:                             existingTokenTypeUnitIDBytes[:],
		Value:                            1000,
		TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
	tokenID := uint256.NewInt(validUnitID)
	tx := &mintFungibleTokenWrapper{wrapper: createWrapper(t, tokenID), attributes: attributes}

	err := handleMintFungibleTokenTx(opts)(tx, 10)
	require.NoError(t, err)

	unit, err := opts.state.GetUnit(tokenID)
	require.NoError(t, err)
	require.NotNil(t, unit)
	require.IsType(t, &fungibleTokenData{}, unit.Data)

	d := unit.Data.(*fungibleTokenData)
	require.Equal(t, attributes.Type, d.tokenType.PaddedBytes(32))
	require.Equal(t, attributes.Value, d.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, uint64(10), d.t)
	require.Equal(t, attributes.Bearer, []byte(unit.Bearer))
}

func TestTransferFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *transferFungibleTokenWrapper
		wantErrStr string
	}{
		{
			name:       "unit ID is 0",
			tx:         &transferFungibleTokenWrapper{wrapper: createWrapper(t, uint256.NewInt(0)), attributes: &TransferFungibleTokenAttributes{}},
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name:       "fungible token does not exists",
			tx:         &transferFungibleTokenWrapper{wrapper: createWrapper(t, uint256.NewInt(42)), attributes: &TransferFungibleTokenAttributes{}},
			wantErrStr: "unit 42 does not exist",
		},

		{
			name:       "unit isn't fungible token",
			tx:         &transferFungibleTokenWrapper{wrapper: createWrapper(t, existingTokenTypeUnitID), attributes: &TransferFungibleTokenAttributes{}},
			wantErrStr: fmt.Sprintf("unit %v is not fungible token data", existingTokenTypeUnitID),
		},
		{
			name: "invalid value",
			tx: &transferFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &TransferFungibleTokenAttributes{
					NewBearer:                    script.PredicateAlwaysTrue(),
					Value:                        existingTokenValue - 1,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid backlink",
			tx: &transferFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &TransferFungibleTokenAttributes{
					NewBearer:                    script.PredicateAlwaysTrue(),
					Value:                        existingTokenValue,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     test.RandomBytes(32),
					InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: "invalid backlink",
		},
		{
			name: "empty token type id",
			tx: &transferFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &TransferFungibleTokenAttributes{
					Type:                         nil,
					NewBearer:                    script.PredicateAlwaysTrue(),
					Value:                        existingTokenValue,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
				}},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token type id",
			tx: &transferFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &TransferFungibleTokenAttributes{
					Type:                         existingTokenTypeUnitIDBytes2,
					NewBearer:                    script.PredicateAlwaysTrue(),
					Value:                        existingTokenValue,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
				}},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: &transferFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &TransferFungibleTokenAttributes{
					Type:                         existingTokenTypeUnitIDBytes,
					NewBearer:                    script.PredicateAlwaysTrue(),
					Value:                        existingTokenValue,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
				}},
			wantErrStr: "script execution result yielded false or non-clean stack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, handleTransferFungibleTokenTx(defaultOpts(t))(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestTransferFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)

	transferAttributes := &TransferFungibleTokenAttributes{
		Type:                         util.Uint256ToBytes(existingTokenTypeUnitID),
		NewBearer:                    script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32)),
		Value:                        existingTokenValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
	uID := uint256.NewInt(existingTokenUnitID)
	tx := &transferFungibleTokenWrapper{wrapper: createWrapper(t, uID), attributes: transferAttributes}

	var roundNr uint64 = 10
	err := handleTransferFungibleTokenTx(opts)(tx, roundNr)
	require.NoError(t, err)

	u, err := opts.state.GetUnit(uID)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data)
	d := u.Data.(*fungibleTokenData)

	require.Equal(t, transferAttributes.NewBearer, []byte(u.Bearer))
	require.Equal(t, transferAttributes.Value, d.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, roundNr, d.t)
}

func TestSplitFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *splitFungibleTokenWrapper
		wantErrStr string
	}{
		{
			name:       "unit ID is 0",
			tx:         &splitFungibleTokenWrapper{wrapper: createWrapper(t, uint256.NewInt(0)), attributes: &SplitFungibleTokenAttributes{}},
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name:       "fungible token does not exists",
			tx:         &splitFungibleTokenWrapper{wrapper: createWrapper(t, uint256.NewInt(42)), attributes: &SplitFungibleTokenAttributes{}},
			wantErrStr: "unit 42 does not exist",
		},
		{
			name:       "unit isn't fungible token",
			tx:         &splitFungibleTokenWrapper{wrapper: createWrapper(t, existingTokenTypeUnitID), attributes: &SplitFungibleTokenAttributes{}},
			wantErrStr: fmt.Sprintf("unit %v is not fungible token data", existingTokenTypeUnitID),
		},
		{
			name: "invalid target value - exceeds the max value",
			tx: &splitFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &SplitFungibleTokenAttributes{
					NewBearer:                    script.PredicateAlwaysTrue(),
					TargetValue:                  existingTokenValue + 1,
					RemainingValue:               1,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: fmt.Sprintf("invalid token value: max allowed %v, got %v", existingTokenValue, existingTokenValue+1),
		},
		{
			name: "invalid value: target + remainder < original value",
			tx: &splitFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &SplitFungibleTokenAttributes{
					NewBearer:                    script.PredicateAlwaysTrue(),
					TargetValue:                  existingTokenValue - 2,
					RemainingValue:               1,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: `remaining value must equal to the original value minus target value`,
		},
		{
			name: "invalid value - remaining value is zero",
			tx: &splitFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &SplitFungibleTokenAttributes{
					Type:                         existingTokenTypeUnitIDBytes,
					NewBearer:                    script.PredicateAlwaysTrue(),
					TargetValue:                  existingTokenValue,
					RemainingValue:               0,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: `when splitting a token the remaining value of the token must be greater than zero`,
		},
		{
			name: "invalid value - target value is zero",
			tx: &splitFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &SplitFungibleTokenAttributes{
					Type:                         existingTokenTypeUnitIDBytes,
					NewBearer:                    script.PredicateAlwaysTrue(),
					RemainingValue:               existingTokenValue,
					TargetValue:                  0,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: `when splitting a token the value assigned to the new token must be greater than zero`,
		},
		{
			name: "invalid backlink",
			tx: &splitFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &SplitFungibleTokenAttributes{
					NewBearer:                    script.PredicateAlwaysTrue(),
					TargetValue:                  existingTokenValue - 1,
					RemainingValue:               1,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     test.RandomBytes(32),
					InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: "invalid backlink",
		},
		{
			name: "empty token type id",
			tx: &splitFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &SplitFungibleTokenAttributes{
					Type:                         nil,
					NewBearer:                    script.PredicateAlwaysTrue(),
					TargetValue:                  existingTokenValue - 1,
					RemainingValue:               1,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
				}},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token type id",
			tx: &splitFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &SplitFungibleTokenAttributes{
					Type:                         existingTokenTypeUnitIDBytes2,
					NewBearer:                    script.PredicateAlwaysTrue(),
					TargetValue:                  existingTokenValue - 1,
					RemainingValue:               1,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
				}},
			wantErrStr: "invalid type identifier",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: &splitFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &SplitFungibleTokenAttributes{
					Type:                         existingTokenTypeUnitIDBytes,
					NewBearer:                    script.PredicateAlwaysTrue(),
					TargetValue:                  existingTokenValue - 1,
					RemainingValue:               1,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
				}},
			wantErrStr: "script execution result yielded false or non-clean stack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, handleSplitFungibleTokenTx(defaultOpts(t))(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestSplitFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)

	var remainingBillValue uint64 = 10
	attr := &SplitFungibleTokenAttributes{
		Type:                         existingTokenTypeUnitIDBytes,
		NewBearer:                    script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32)),
		TargetValue:                  existingTokenValue - remainingBillValue,
		RemainingValue:               remainingBillValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
	uID := uint256.NewInt(existingTokenUnitID)
	tx := &splitFungibleTokenWrapper{wrapper: createWrapper(t, uID), attributes: attr}
	var roundNr uint64 = 10
	err := handleSplitFungibleTokenTx(opts)(tx, roundNr)
	require.NoError(t, err)

	u, err := opts.state.GetUnit(uID)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data)
	d := u.Data.(*fungibleTokenData)

	require.Equal(t, script.PredicateAlwaysTrue(), []byte(u.Bearer))
	require.Equal(t, remainingBillValue, d.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, roundNr, d.t)

	newUnitID := txutil.SameShardID(uID, tx.HashForIDCalculation(opts.hashAlgorithm))
	newUnit, err := opts.state.GetUnit(newUnitID)
	require.NoError(t, err)
	require.NotNil(t, newUnit)
	require.IsType(t, &fungibleTokenData{}, newUnit.Data)

	newUnitData := newUnit.Data.(*fungibleTokenData)

	require.Equal(t, attr.NewBearer, []byte(newUnit.Bearer))
	require.Equal(t, existingTokenValue-remainingBillValue, newUnitData.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), newUnitData.backlink)
	require.Equal(t, roundNr, newUnitData.t)
}

func TestBurnFungibleToken_NotOk(t *testing.T) {
	tests := []struct {
		name       string
		tx         *burnFungibleTokenWrapper
		wantErrStr string
	}{
		{
			name:       "unit ID is 0",
			tx:         &burnFungibleTokenWrapper{wrapper: createWrapper(t, uint256.NewInt(0)), attributes: &BurnFungibleTokenAttributes{}},
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name:       "fungible token does not exists",
			tx:         &burnFungibleTokenWrapper{wrapper: createWrapper(t, uint256.NewInt(42)), attributes: &BurnFungibleTokenAttributes{}},
			wantErrStr: "unit 42 does not exist",
		},
		{
			name:       "unit isn't fungible token",
			tx:         &burnFungibleTokenWrapper{wrapper: createWrapper(t, existingTokenTypeUnitID), attributes: &BurnFungibleTokenAttributes{}},
			wantErrStr: fmt.Sprintf("unit %v is not fungible token data", existingTokenTypeUnitID),
		},
		{
			name: "invalid value",
			tx: &burnFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &BurnFungibleTokenAttributes{
					Type:                         existingTokenTypeUnitIDBytes[:],
					Value:                        existingTokenValue - 1,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid backlink",
			tx: &burnFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &BurnFungibleTokenAttributes{
					Type:                         existingTokenTypeUnitIDBytes[:],
					Value:                        existingTokenValue,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     test.RandomBytes(32),
					InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
				}},
			wantErrStr: "invalid backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: &burnFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &BurnFungibleTokenAttributes{
					Type:                         existingTokenTypeUnitIDBytes[:],
					Value:                        existingTokenValue,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
				}},
			wantErrStr: "script execution result yielded false or non-clean stack",
		},
		{
			name: "invalid token type",
			tx: &burnFungibleTokenWrapper{
				wrapper: createWrapper(t, uint256.NewInt(existingTokenUnitID)),
				attributes: &BurnFungibleTokenAttributes{
					Type: func() []byte {
						r := uint256.NewInt(42).Bytes32()
						return r[:]
					}(),
					Value:                        existingTokenValue,
					Nonce:                        test.RandomBytes(32),
					Backlink:                     make([]byte, 32),
					InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
				}},
			wantErrStr: "type of token to burn does not matches the actual type of the token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, handleBurnFungibleTokenTx(defaultOpts(t))(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestBurnFungibleToken_Ok(t *testing.T) {
	opts := defaultOpts(t)
	burnAttributes := &BurnFungibleTokenAttributes{
		Type:                         existingTokenTypeUnitIDBytes[:],
		Value:                        existingTokenValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
	uID := uint256.NewInt(existingTokenUnitID)
	tx := &burnFungibleTokenWrapper{wrapper: createWrapper(t, uID), attributes: burnAttributes}
	var roundNr uint64 = 10
	err := handleBurnFungibleTokenTx(opts)(tx, roundNr)
	require.NoError(t, err)

	u, err := opts.state.GetUnit(uID)
	require.Nil(t, u)
	require.ErrorContains(t, err, fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(uID)))
}

func TestJoinFungibleToken_NotOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}

	burnTxInvalidSource := createTx(t, uint256.NewInt(existingTokenUnitID), &BurnFungibleTokenAttributes{
		Type:                         existingTokenTypeUnitIDBytes[:],
		Value:                        existingTokenValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	})
	burnTx := createTx(t, uint256.NewInt(existingTokenUnitID), &BurnFungibleTokenAttributes{
		Type:                         existingTokenTypeUnitIDBytes[:],
		Value:                        existingTokenValue,
		Nonce:                        make([]byte, 32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	})
	burnTx2 := createTx(t, uint256.NewInt(existingTokenUnitID2), &BurnFungibleTokenAttributes{
		Type:                         existingTokenTypeUnitIDBytes2[:],
		Value:                        existingTokenValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	})
	maxUintValueTokenID := uint64(existingTokenUnitID2 + 1)
	burnTx3 := createTx(t, uint256.NewInt(maxUintValueTokenID), &BurnFungibleTokenAttributes{
		Type:                         existingTokenTypeUnitIDBytes2[:],
		Value:                        math.MaxUint64,
		Nonce:                        make([]byte, 32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	})
	proofInvalidSource := testblock.CreateProof(t, burnTxInvalidSource, signer, util.Uint256ToBytes(uint256.NewInt(existingTokenUnitID)))
	proofBurnTx2 := testblock.CreateProof(t, burnTx2, signer, util.Uint256ToBytes(uint256.NewInt(existingTokenUnitID2)))
	proofBurnTx3 := testblock.CreateProof(t, burnTx3, signer, util.Uint256ToBytes(uint256.NewInt(maxUintValueTokenID)))
	emptyBlockProof := testblock.CreateProof(t, nil, signer, util.Uint256ToBytes(uint256.NewInt(existingTokenUnitID)))

	tests := []struct {
		name       string
		tx         txsystem.GenericTransaction
		wantErrStr string
	}{
		{
			name:       "unit ID is 0",
			tx:         createTx(t, uint256.NewInt(0), &JoinFungibleTokenAttributes{}),
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTx(t, uint256.NewInt(42), &JoinFungibleTokenAttributes{}),
			wantErrStr: "unit 42 does not exist",
		},

		{
			name:       "unit isn't fungible token",
			tx:         createTx(t, existingTokenTypeUnitID, &JoinFungibleTokenAttributes{}),
			wantErrStr: fmt.Sprintf("unit %v is not fungible token data", existingTokenTypeUnitID),
		},
		{
			name: "invalid backlink",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &JoinFungibleTokenAttributes{

				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}),
			wantErrStr: "invalid backlink",
		},
		{
			name: "source not burned",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &JoinFungibleTokenAttributes{
				BurnTransactions:             []*txsystem.Transaction{burnTxInvalidSource.ToProtoBuf()},
				Proofs:                       []*block.BlockProof{proofInvalidSource},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}),
			wantErrStr: "the source tokens weren't burned to join them to the target token",
		},
		{
			name: "invalid source token type",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &JoinFungibleTokenAttributes{
				BurnTransactions:             []*txsystem.Transaction{burnTx2.ToProtoBuf()},
				Proofs:                       []*block.BlockProof{proofInvalidSource},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}),
			wantErrStr: "the type of the burned source token does not match the type of target token",
		},
		{
			name: "invalid proof type",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &JoinFungibleTokenAttributes{
				BurnTransactions:             []*txsystem.Transaction{burnTx.ToProtoBuf()},
				Proofs:                       []*block.BlockProof{emptyBlockProof},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}),
			wantErrStr: "invalid proof type",
		},
		{
			name: "proof is not valid",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &JoinFungibleTokenAttributes{
				BurnTransactions:             []*txsystem.Transaction{burnTx.ToProtoBuf()},
				Proofs:                       []*block.BlockProof{proofBurnTx2},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}),
			wantErrStr: "proof is not valid",
		},
		{
			name: "uint64 overflow",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID2), &JoinFungibleTokenAttributes{
				BurnTransactions:             []*txsystem.Transaction{burnTx3.ToProtoBuf()},
				Proofs:                       []*block.BlockProof{proofBurnTx3},
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}),
			wantErrStr: "invalid sum of tokens: uint64 overflow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			joinTx, ok := tt.tx.(*joinFungibleTokenWrapper)
			require.True(t, ok, "invalid transaction type %T", tt.tx)

			require.ErrorContains(t, handleJoinFungibleTokenTx(opts)(joinTx, 10), tt.wantErrStr)
		})
	}
}

func defaultOpts(t *testing.T) *Options {
	o, err := defaultOptions()
	o.state = initState(t)
	require.NoError(t, err)
	return o
}

func initState(t *testing.T) *rma.Tree {
	state, err := rma.New(&rma.Config{
		HashAlgorithm: gocrypto.SHA256,
	})
	require.NoError(t, err)
	err = state.AtomicUpdate(rma.AddItem(existingTokenTypeUnitID, script.PredicateAlwaysTrue(), &fungibleTokenTypeData{
		symbol:                   "ALPHA",
		name:                     "A long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		parentTypeId:             uint256.NewInt(0),
		decimalPlaces:            5,
		subTypeCreationPredicate: script.PredicateAlwaysTrue(),
		tokenCreationPredicate:   script.PredicateAlwaysTrue(),
		invariantPredicate:       script.PredicateAlwaysTrue(),
	}, make([]byte, 32)))
	require.NoError(t, err)
	err = state.AtomicUpdate(rma.AddItem(existingTokenTypeUnitID2, script.PredicateAlwaysTrue(), &fungibleTokenTypeData{
		symbol:                   "ALPHA2",
		name:                     "A long name for ALPHA2",
		icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		parentTypeId:             uint256.NewInt(0),
		decimalPlaces:            5,
		subTypeCreationPredicate: script.PredicateAlwaysTrue(),
		tokenCreationPredicate:   script.PredicateAlwaysTrue(),
		invariantPredicate:       script.PredicateAlwaysTrue(),
	}, make([]byte, 32)))
	require.NoError(t, err)
	err = state.AtomicUpdate(rma.AddItem(uint256.NewInt(existingTokenUnitID), script.PredicateAlwaysTrue(), &fungibleTokenData{
		tokenType: existingTokenTypeUnitID,
		value:     existingTokenValue,
		t:         0,
		backlink:  make([]byte, 32),
	}, make([]byte, 32)))
	require.NoError(t, err)
	err = state.AtomicUpdate(rma.AddItem(uint256.NewInt(existingTokenUnitID2), script.PredicateAlwaysTrue(), &fungibleTokenData{
		tokenType: existingTokenTypeUnitID2,
		value:     existingTokenValue,
		t:         0,
		backlink:  make([]byte, 32),
	}, make([]byte, 32)))
	require.NoError(t, err)
	err = state.AtomicUpdate(rma.AddItem(feeCreditID, script.PredicateAlwaysTrue(), &fc.FeeCreditRecord{
		Balance: 100,
		Hash:    make([]byte, 32),
		Timeout: 100,
	}, make([]byte, 32)))
	require.NoError(t, err)
	return state
}

func createTx(t *testing.T, unitID *uint256.Int, attributes proto.Message) txsystem.GenericTransaction {
	id := unitID.Bytes32()
	return testtransaction.NewGenericTransaction(
		t,
		NewGenericTx,
		testtransaction.WithUnitId(id[:]),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
		testtransaction.WithAttributes(attributes),
	)
}

func createWrapper(t *testing.T, unitID *uint256.Int) wrapper {
	id := unitID.Bytes32()
	return wrapper{
		transaction: testtransaction.NewTransaction(
			t,
			testtransaction.WithUnitId(id[:]),
			testtransaction.WithSystemID(DefaultSystemIdentifier),
			testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
			testtransaction.WithFeeProof(script.PredicateArgumentEmpty()),
			testtransaction.WithClientMetadata(&txsystem.ClientMetadata{
				Timeout:           1000,
				MaxFee:            10,
				FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
			}),
		),
	}
}
