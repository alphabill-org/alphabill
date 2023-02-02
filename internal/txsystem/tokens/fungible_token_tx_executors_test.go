package tokens

import (
	gocrypto "crypto"
	"fmt"
	"math"
	"testing"

	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const (
	invalidSymbolName = "♥♥♥♥♥♥♥♥ We ♥ Alphabill ♥♥♥♥♥♥♥♥"
	validSymbolName   = "BETA"
	validUnitID       = 0x0000000000000000000000000000000000000000000000000000000000000064

	existingTokenUnitID = 2
	existingTokenValue  = 1000

	existingTokenUnitID2 = 10000
)

var (
	existingTokenTypeUnitID      = uint256.NewInt(1)
	existingTokenTypeUnitIDBytes = existingTokenTypeUnitID.Bytes32()

	existingTokenTypeUnitID2      = uint256.NewInt(1001)
	existingTokenTypeUnitIDBytes2 = existingTokenTypeUnitID2.Bytes32()
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
			tx:         createTx(t, existingTokenTypeUnitID, &CreateFungibleTokenTypeAttributes{Symbol: validSymbolName, DecimalPlaces: 5}),
			wantErrStr: fmt.Sprintf("unit %v exists", existingTokenTypeUnitID),
		},
		{
			name:       "parent.decimals != tx.attributes.decimalPlaces",
			tx:         createTx(t, uint256.NewInt(validUnitID), &CreateFungibleTokenTypeAttributes{Symbol: validSymbolName, DecimalPlaces: 6, ParentTypeId: existingTokenTypeUnitIDBytes[:]}),
			wantErrStr: "invalid decimal places. allowed 5, got 6",
		},
		{
			name:       "parent does not exist",
			tx:         createTx(t, uint256.NewInt(validUnitID), &CreateFungibleTokenTypeAttributes{Symbol: validSymbolName, DecimalPlaces: 6, ParentTypeId: util.Uint256ToBytes(uint256.NewInt(100))}),
			wantErrStr: fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(uint256.NewInt(validUnitID))),
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
		Symbol:                             validSymbolName + "_CHILD",
		ParentTypeId:                       util.Uint256ToBytes(parentID),
		DecimalPlaces:                      6,
		SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
		TokenCreationPredicate:             script.PredicateAlwaysTrue(),
		InvariantPredicate:                 script.PredicateAlwaysTrue(),
		SubTypeCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
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

func TestCreateFungibleTokenType_CreateTokenTypeChain_InvalidCreationPredicateSignature(t *testing.T) {
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
	parentIDBytes := parentID.Bytes32()
	childID := uint256.NewInt(20)
	childAttributes := &CreateFungibleTokenTypeAttributes{
		Symbol:                             validSymbolName + "_CHILD",
		ParentTypeId:                       parentIDBytes[:],
		DecimalPlaces:                      6,
		SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
		TokenCreationPredicate:             script.PredicateAlwaysTrue(),
		InvariantPredicate:                 script.PredicateAlwaysTrue(),
		SubTypeCreationPredicateSignatures: [][]byte{[]byte("invalid")},
	}

	err := executor.Execute(createTx(t, parentID, parentAttributes), 10)
	require.NoError(t, err)

	err = executor.Execute(createTx(t, childID, childAttributes), 11)
	require.ErrorContains(t, err, "invalid script format")
}

func TestMintFungibleToken_NotOk(t *testing.T) {
	executor := &mintFungibleTokenTxExecutor{
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
			tx:         createTx(t, uint256.NewInt(0), &MintFungibleTokenAttributes{}),
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name:       "unit with given ID exists",
			tx:         createTx(t, existingTokenTypeUnitID, &MintFungibleTokenAttributes{}),
			wantErrStr: fmt.Sprintf("unit %v exists", existingTokenTypeUnitID),
		},
		{
			name: "parent does not exist",
			tx: createTx(t, uint256.NewInt(validUnitID), &MintFungibleTokenAttributes{
				Bearer:                           script.PredicateAlwaysTrue(),
				Type:                             util.Uint256ToBytes(uint256.NewInt(100)),
				Value:                            1000,
				TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}),
			wantErrStr: fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(uint256.NewInt(validUnitID))),
		},
		{
			name: "invalid token creation predicate argument",
			tx: createTx(t, uint256.NewInt(validUnitID), &MintFungibleTokenAttributes{
				Bearer:                           script.PredicateAlwaysTrue(),
				Type:                             existingTokenTypeUnitIDBytes[:],
				Value:                            1000,
				TokenCreationPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}),
			wantErrStr: "script execution result yielded false or non-clean stack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, executor.Execute(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestMintFungibleToken_Ok(t *testing.T) {
	executor := &mintFungibleTokenTxExecutor{
		baseTxExecutor: &baseTxExecutor[*fungibleTokenTypeData]{
			state:         initState(t),
			hashAlgorithm: gocrypto.SHA256,
		},
	}
	attributes := &MintFungibleTokenAttributes{
		Bearer:                           script.PredicateAlwaysTrue(),
		Type:                             existingTokenTypeUnitIDBytes[:],
		Value:                            1000,
		TokenCreationPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
	tokenID := uint256.NewInt(validUnitID)
	tx := createTx(t, tokenID, attributes)
	err := executor.Execute(tx, 10)
	require.NoError(t, err)

	unit, err := executor.state.GetUnit(tokenID)
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
	executor := &transferFungibleTokenTxExecutor{
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
			tx:         createTx(t, uint256.NewInt(0), &TransferFungibleTokenAttributes{}),
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTx(t, uint256.NewInt(42), &TransferFungibleTokenAttributes{}),
			wantErrStr: "unit 42 does not exist",
		},

		{
			name:       "unit isn't fungible token",
			tx:         createTx(t, existingTokenTypeUnitID, &TransferFungibleTokenAttributes{}),
			wantErrStr: fmt.Sprintf("unit %v is not fungible token data", existingTokenTypeUnitID),
		},
		{
			name: "invalid value",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &TransferFungibleTokenAttributes{
				NewBearer:                    script.PredicateAlwaysTrue(),
				Value:                        existingTokenValue - 1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}),
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid backlink",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &TransferFungibleTokenAttributes{
				NewBearer:                    script.PredicateAlwaysTrue(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}),
			wantErrStr: "invalid backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &TransferFungibleTokenAttributes{
				NewBearer:                    script.PredicateAlwaysTrue(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}),
			wantErrStr: "script execution result yielded false or non-clean stack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, executor.Execute(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestTransferFungibleToken_Ok(t *testing.T) {
	executor := &transferFungibleTokenTxExecutor{
		baseTxExecutor: &baseTxExecutor[*fungibleTokenTypeData]{
			state:         initState(t),
			hashAlgorithm: gocrypto.SHA256,
		},
	}

	transferAttributes := &TransferFungibleTokenAttributes{
		NewBearer:                    script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32)),
		Value:                        existingTokenValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}

	uID := uint256.NewInt(existingTokenUnitID)
	tx := createTx(t, uID, transferAttributes)
	var roundNr uint64 = 10
	err := executor.Execute(tx, roundNr)
	require.NoError(t, err)

	u, err := executor.state.GetUnit(uID)
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
	executor := &splitFungibleTokenTxExecutor{
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
			tx:         createTx(t, uint256.NewInt(0), &SplitFungibleTokenAttributes{}),
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTx(t, uint256.NewInt(42), &SplitFungibleTokenAttributes{}),
			wantErrStr: "unit 42 does not exist",
		},

		{
			name:       "unit isn't fungible token",
			tx:         createTx(t, existingTokenTypeUnitID, &SplitFungibleTokenAttributes{}),
			wantErrStr: fmt.Sprintf("unit %v is not fungible token data", existingTokenTypeUnitID),
		},
		{
			name: "invalid value",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &SplitFungibleTokenAttributes{
				NewBearer:                    script.PredicateAlwaysTrue(),
				TargetValue:                  existingTokenValue + 1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}),
			wantErrStr: fmt.Sprintf("invalid token value: max allowed %v, got %v", existingTokenValue, existingTokenValue+1),
		},
		{
			name: "invalid backlink",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &SplitFungibleTokenAttributes{
				NewBearer:                    script.PredicateAlwaysTrue(),
				TargetValue:                  existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}),
			wantErrStr: "invalid backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &SplitFungibleTokenAttributes{
				NewBearer:                    script.PredicateAlwaysTrue(),
				TargetValue:                  existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}),
			wantErrStr: "script execution result yielded false or non-clean stack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, executor.Execute(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestSplitFungibleToken_Ok(t *testing.T) {
	executor := &splitFungibleTokenTxExecutor{
		baseTxExecutor: &baseTxExecutor[*fungibleTokenTypeData]{
			state:         initState(t),
			hashAlgorithm: gocrypto.SHA256,
		},
	}

	var remainingValue uint64 = 10
	transferAttributes := &SplitFungibleTokenAttributes{
		NewBearer:                    script.PredicatePayToPublicKeyHashDefault(test.RandomBytes(32)),
		TargetValue:                  existingTokenValue - remainingValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}

	uID := uint256.NewInt(existingTokenUnitID)
	tx := createTx(t, uID, transferAttributes)
	var roundNr uint64 = 10
	err := executor.Execute(tx, roundNr)
	require.NoError(t, err)

	u, err := executor.state.GetUnit(uID)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data)
	d := u.Data.(*fungibleTokenData)

	require.Equal(t, script.PredicateAlwaysTrue(), []byte(u.Bearer))
	require.Equal(t, remainingValue, d.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, roundNr, d.t)

	newUnitID := txutil.SameShardID(uID, tx.(*splitFungibleTokenWrapper).HashForIDCalculation(executor.hashAlgorithm))
	newUnit, err := executor.state.GetUnit(newUnitID)
	require.NoError(t, err)
	require.NotNil(t, newUnit)
	require.IsType(t, &fungibleTokenData{}, newUnit.Data)

	newUnitData := newUnit.Data.(*fungibleTokenData)

	require.Equal(t, transferAttributes.NewBearer, []byte(newUnit.Bearer))
	require.Equal(t, existingTokenValue-remainingValue, newUnitData.value)
	require.Equal(t, tx.Hash(gocrypto.SHA256), newUnitData.backlink)
	require.Equal(t, roundNr, newUnitData.t)
}

func TestBurnFungibleToken_NotOk(t *testing.T) {
	executor := &burnFungibleTokenTxExecutor{
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
			tx:         createTx(t, uint256.NewInt(0), &BurnFungibleTokenAttributes{}),
			wantErrStr: "unit ID cannot be zero",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTx(t, uint256.NewInt(42), &BurnFungibleTokenAttributes{}),
			wantErrStr: "unit 42 does not exist",
		},
		{
			name:       "unit isn't fungible token",
			tx:         createTx(t, existingTokenTypeUnitID, &BurnFungibleTokenAttributes{}),
			wantErrStr: fmt.Sprintf("unit %v is not fungible token data", existingTokenTypeUnitID),
		},
		{
			name: "invalid value",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &BurnFungibleTokenAttributes{
				Type:                         existingTokenTypeUnitIDBytes[:],
				Value:                        existingTokenValue - 1,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}),
			wantErrStr: fmt.Sprintf("invalid token value: expected %v, got %v", existingTokenValue, existingTokenValue-1),
		},
		{
			name: "invalid backlink",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &BurnFungibleTokenAttributes{
				Type:                         existingTokenTypeUnitIDBytes[:],
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}),
			wantErrStr: "invalid backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &BurnFungibleTokenAttributes{
				Type:                         existingTokenTypeUnitIDBytes[:],
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}),
			wantErrStr: "script execution result yielded false or non-clean stack",
		},
		{
			name: "invalid token type",
			tx: createTx(t, uint256.NewInt(existingTokenUnitID), &BurnFungibleTokenAttributes{
				Type: func() []byte {
					r := uint256.NewInt(42).Bytes32()
					return r[:]
				}(),
				Value:                        existingTokenValue,
				Nonce:                        test.RandomBytes(32),
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}),
			wantErrStr: "type of token to burn does not matches the actual type of the token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, executor.Execute(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func TestBurnFungibleToken_Ok(t *testing.T) {
	executor := &burnFungibleTokenTxExecutor{
		baseTxExecutor: &baseTxExecutor[*fungibleTokenTypeData]{
			state:         initState(t),
			hashAlgorithm: gocrypto.SHA256,
		},
	}

	burnAttributes := &BurnFungibleTokenAttributes{
		Type:                         existingTokenTypeUnitIDBytes[:],
		Value:                        existingTokenValue,
		Nonce:                        test.RandomBytes(32),
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}

	uID := uint256.NewInt(existingTokenUnitID)
	tx := createTx(t, uID, burnAttributes)
	var roundNr uint64 = 10
	err := executor.Execute(tx, roundNr)
	require.NoError(t, err)

	u, err := executor.state.GetUnit(uID)
	require.Nil(t, u)
	require.ErrorContains(t, err, fmt.Sprintf("item %X does not exist", util.Uint256ToBytes(uID)))
}

func TestJoinFungibleToken_NotOk(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
	executor := &joinFungibleTokenTxExecutor{
		baseTxExecutor: &baseTxExecutor[*fungibleTokenTypeData]{
			state:         initState(t),
			hashAlgorithm: gocrypto.SHA256,
		},
		trustBase: trustBase,
	}

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
			name:       "invalid tx type",
			tx:         &createNonFungibleTokenTypeWrapper{},
			wantErrStr: fmt.Sprintf("invalid tx type: %T", &createNonFungibleTokenTypeWrapper{}),
		},
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
			require.ErrorContains(t, executor.Execute(tt.tx, 10), tt.wantErrStr)
		})
	}
}

func initState(t *testing.T) *rma.Tree {
	state, err := rma.New(&rma.Config{
		HashAlgorithm: gocrypto.SHA256,
	})
	require.NoError(t, err)
	err = state.AtomicUpdate(rma.AddItem(existingTokenTypeUnitID, script.PredicateAlwaysTrue(), &fungibleTokenTypeData{
		symbol:                   "ALPHA",
		parentTypeId:             uint256.NewInt(0),
		decimalPlaces:            5,
		subTypeCreationPredicate: script.PredicateAlwaysTrue(),
		tokenCreationPredicate:   script.PredicateAlwaysTrue(),
		invariantPredicate:       script.PredicateAlwaysTrue(),
	}, make([]byte, 32)))
	require.NoError(t, err)
	err = state.AtomicUpdate(rma.AddItem(existingTokenTypeUnitID2, script.PredicateAlwaysTrue(), &fungibleTokenTypeData{
		symbol:                   "ALPHA2",
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
	return state
}

func createTx(t *testing.T, unitID *uint256.Int, attributes proto.Message) txsystem.GenericTransaction {
	id := unitID.Bytes32()
	return testtransaction.NewGenericTransaction(
		t,
		NewGenericTx,
		testtransaction.WithUnitId(id[:]),
		testtransaction.WithSystemID(DefaultTokenTxSystemIdentifier),
		testtransaction.WithAttributes(attributes),
	)
}
