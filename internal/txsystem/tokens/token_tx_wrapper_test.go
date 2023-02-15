package tokens

import (
	"bytes"
	gocrypto "crypto"
	"testing"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

var (
	systemID                                  = []byte{0, 0, 4, 2}
	unitID                                    = []byte{1}
	ownerProof                                = []byte{2}
	symbol                                    = "TEST"
	parentTypeId                              = []byte{3}
	subTypeCreationPredicate                  = []byte{4}
	tokenCreationPredicate                    = []byte{5}
	invariantPredicate                        = []byte{6}
	dataUpdatePredicate                       = []byte{7}
	subTypeCreationPredicateSignatures        = [][]byte{{8}, {8}}
	timeout                                   = uint64(100)
	bearer                                    = []byte{10}
	nftType                                   = []byte{11}
	data                                      = []byte{12}
	updatedData                               = []byte{0, 12}
	uri                                       = "https://alphabill.org"
	tokenCreationPredicateSignatures          = [][]byte{{14}, {14}}
	newBearer                                 = []byte{15}
	nonce                                     = []byte{16}
	backlink                                  = []byte{17}
	invariantPredicateSignatures              = [][]byte{{18}, {18}}
	dataUpdateSignatures                      = [][]byte{{19}, {19}}
	fungibleTokenDecimalPlaces         uint32 = 8
	fungibleTokenValue                 uint64 = 100_000_002
	transferValue                      uint64 = 1000
	remainingValue                     uint64 = 1000
	burnValue                          uint64 = 2000
	burnType                                  = []byte{20}
)

func TestCreateNonFungibleTokenTypeTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createNonFungibleTokenTypeTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	sigBytes := genericTx.SigBytes()

	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0))
	b.Write(nil)
	b.Write([]byte(symbol))
	b.Write(parentTypeId)
	b.Write(subTypeCreationPredicate)
	b.Write(tokenCreationPredicate)
	b.Write(invariantPredicate)
	b.Write(dataUpdatePredicate)
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestMintNonFungibleTokenTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createMintNonFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	sigBytes := genericTx.SigBytes()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0))
	b.Write(nil)
	b.Write(bearer)
	b.Write(nftType)
	b.Write([]byte(uri))
	b.Write(data)
	b.Write(dataUpdatePredicate)
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestCreateNonFungibleTokenTypeTx_GetHashIsCalculatedCorrectly(t *testing.T) {
	tx := createNonFungibleTokenTypeTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	hash := genericTx.Hash(gocrypto.SHA256)

	b := gocrypto.SHA256.New()
	b.Write(systemID)
	b.Write(unitID)
	b.Write(ownerProof)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(1)) // server metadata fee
	b.Write([]byte(symbol))
	b.Write(parentTypeId)
	b.Write(subTypeCreationPredicate)
	b.Write(tokenCreationPredicate)
	b.Write(invariantPredicate)
	b.Write(dataUpdatePredicate)
	for _, sig := range subTypeCreationPredicateSignatures {
		b.Write(sig)
	}
	require.Equal(t, b.Sum(nil), hash)
}

func TestMintNonFungibleTokenTx_GetHashIsCalculatedCorrectly(t *testing.T) {
	tx := createMintNonFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	hash := genericTx.Hash(gocrypto.SHA256)

	b := gocrypto.SHA256.New()
	b.Write(systemID)
	b.Write(unitID)
	b.Write(ownerProof)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(1)) // server metadata fee
	b.Write(bearer)
	b.Write(nftType)
	b.Write([]byte(uri))
	b.Write(data)
	b.Write(dataUpdatePredicate)
	for _, sig := range tokenCreationPredicateSignatures {
		b.Write(sig)
	}
	require.Equal(t, b.Sum(nil), hash)
}

func TestTransferNonFungibleTokenTypeTx_GetHashIsCalculatedCorrectly(t *testing.T) {
	tx := createTransferNonFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	hash := genericTx.Hash(gocrypto.SHA256)

	b := gocrypto.SHA256.New()
	b.Write(systemID)
	b.Write(unitID)
	b.Write(ownerProof)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(1)) // server metadata fee
	b.Write(newBearer)
	b.Write(nonce)
	b.Write(backlink)
	for _, sig := range invariantPredicateSignatures {
		b.Write(sig)
	}
	b.Write(nftType)
	require.Equal(t, b.Sum(nil), hash)
}

func TestTransferNonFungibleTokenTypeTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createTransferNonFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	sigBytes := genericTx.SigBytes()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0))
	b.Write(nil)
	b.Write(newBearer)
	b.Write(nonce)
	b.Write(backlink)
	b.Write(nftType)
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestUpdateNonFungibleTokenTypeTx_GetHashIsCalculatedCorrectly(t *testing.T) {
	tx := createUpdateNonFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	hash := genericTx.Hash(gocrypto.SHA256)

	b := gocrypto.SHA256.New()
	b.Write(systemID)
	b.Write(unitID)
	b.Write(ownerProof)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(1)) // server metadata fee
	b.Write(updatedData)
	b.Write(backlink)
	for _, sig := range dataUpdateSignatures {
		b.Write(sig)
	}
	require.Equal(t, b.Sum(nil), hash)
}

func TestUpdateNonFungibleTokenTypeTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createUpdateNonFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	sigBytes := genericTx.SigBytes()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0))
	b.Write(nil)
	b.Write(updatedData)
	b.Write(backlink)
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestCreateFungibleTokenTypeTx_GetHashIsCalculatedCorrectly(t *testing.T) {
	tx := createFungibleTokenTypeTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	hash := genericTx.Hash(gocrypto.SHA256)

	b := gocrypto.SHA256.New()
	b.Write(systemID)
	b.Write(unitID)
	b.Write(ownerProof)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(1)) // server metadata fee
	b.Write([]byte(symbol))
	b.Write(parentTypeId)
	b.Write(util.Uint32ToBytes(fungibleTokenDecimalPlaces))
	b.Write(subTypeCreationPredicate)
	b.Write(tokenCreationPredicate)
	b.Write(invariantPredicate)
	for _, sig := range subTypeCreationPredicateSignatures {
		b.Write(sig)
	}
	require.Equal(t, b.Sum(nil), hash)
}

func TestCreateFungibleTokenTypeTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createFungibleTokenTypeTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	sigBytes := genericTx.SigBytes()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0))
	b.Write(nil)
	b.Write([]byte(symbol))
	b.Write(parentTypeId)
	b.Write(util.Uint32ToBytes(fungibleTokenDecimalPlaces))
	b.Write(subTypeCreationPredicate)
	b.Write(tokenCreationPredicate)
	b.Write(invariantPredicate)
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestMintFungibleTokenTx_GetHashIsCalculatedCorrectly(t *testing.T) {
	tx := mintFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	hash := genericTx.Hash(gocrypto.SHA256)

	b := gocrypto.SHA256.New()
	b.Write(systemID)
	b.Write(unitID)
	b.Write(ownerProof)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(1)) // server metadata fee
	b.Write(bearer)
	b.Write(parentTypeId)
	b.Write(util.Uint64ToBytes(fungibleTokenValue))
	for _, sig := range tokenCreationPredicateSignatures {
		b.Write(sig)
	}
	require.Equal(t, b.Sum(nil), hash)
}

func TestMintFungibleTokenTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := mintFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	sigBytes := genericTx.SigBytes()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0))
	b.Write(nil)
	b.Write(bearer)
	b.Write(parentTypeId)
	b.Write(util.Uint64ToBytes(fungibleTokenValue))
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestTransferFungibleTokenTx_GetHashIsCalculatedCorrectly(t *testing.T) {
	tx := transferFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	hash := genericTx.Hash(gocrypto.SHA256)

	b := gocrypto.SHA256.New()
	b.Write(systemID)
	b.Write(unitID)
	b.Write(ownerProof)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(1)) // server metadata fee
	b.Write(newBearer)
	b.Write(util.Uint64ToBytes(transferValue))
	b.Write(nonce)
	b.Write(backlink)
	for _, sig := range invariantPredicateSignatures {
		b.Write(sig)
	}
	b.Write(parentTypeId)
	require.Equal(t, b.Sum(nil), hash)
}

func TestTransferFungibleTokenTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := transferFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	sigBytes := genericTx.SigBytes()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0))
	b.Write(nil)
	b.Write(newBearer)
	b.Write(util.Uint64ToBytes(transferValue))
	b.Write(nonce)
	b.Write(backlink)
	b.Write(parentTypeId)
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestSplitFungibleTokenTx_GetHashIsCalculatedCorrectly(t *testing.T) {
	tx := splitFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	hash := genericTx.Hash(gocrypto.SHA256)

	b := gocrypto.SHA256.New()
	b.Write(systemID)
	b.Write(unitID)
	b.Write(ownerProof)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(1)) // server metadata fee
	b.Write(newBearer)
	b.Write(util.Uint64ToBytes(transferValue))
	b.Write(util.Uint64ToBytes(remainingValue))
	b.Write(nonce)
	b.Write(backlink)
	for _, sig := range invariantPredicateSignatures {
		b.Write(sig)
	}
	b.Write(parentTypeId)
	require.Equal(t, b.Sum(nil), hash)
}

func TestSplitFungibleTokenTx_TargetUnitsReturnsTwoUnits(t *testing.T) {
	tx := splitFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)

	units := genericTx.TargetUnits(gocrypto.SHA256)
	require.Len(t, units, 2)
	require.Equal(t, genericTx.UnitID(), units[0])
	splitWrapper := genericTx.(*splitFungibleTokenWrapper)
	sameShardId := utiltx.SameShardID(genericTx.UnitID(), splitWrapper.HashForIDCalculation(gocrypto.SHA256))
	require.Equal(t, sameShardId, units[1])
}

func TestSplitFungibleTokenTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := splitFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	sigBytes := genericTx.SigBytes()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0))
	b.Write(nil)
	b.Write(newBearer)
	b.Write(util.Uint64ToBytes(transferValue))
	b.Write(util.Uint64ToBytes(remainingValue))
	b.Write(nonce)
	b.Write(backlink)
	b.Write(parentTypeId)
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestBurnFungibleTokenTx_GetHashIsCalculatedCorrectly(t *testing.T) {
	tx := burnFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	hash := genericTx.Hash(gocrypto.SHA256)

	b := gocrypto.SHA256.New()
	b.Write(systemID)
	b.Write(unitID)
	b.Write(ownerProof)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(1)) // server metadata fee
	b.Write(burnType)
	b.Write(util.Uint64ToBytes(burnValue))
	b.Write(nonce)
	b.Write(backlink)
	for _, sig := range invariantPredicateSignatures {
		b.Write(sig)
	}
	require.Equal(t, b.Sum(nil), hash)
}

func TestBurnFungibleTokenTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := burnFungibleTokenTxOrder(t, systemID)
	genericTx, err := NewGenericTx(tx)
	require.NoError(t, err)
	sigBytes := genericTx.SigBytes()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0))
	b.Write(nil)
	b.Write(burnType)
	b.Write(util.Uint64ToBytes(burnValue))
	b.Write(nonce)
	b.Write(backlink)
	require.Equal(t, b.Bytes(), sigBytes)
}

func createFungibleTokenTypeTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: timeout}),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&CreateFungibleTokenTypeAttributes{
			Symbol:                             symbol,
			ParentTypeId:                       parentTypeId,
			DecimalPlaces:                      fungibleTokenDecimalPlaces,
			SubTypeCreationPredicate:           subTypeCreationPredicate,
			TokenCreationPredicate:             tokenCreationPredicate,
			InvariantPredicate:                 invariantPredicate,
			SubTypeCreationPredicateSignatures: subTypeCreationPredicateSignatures,
		}),
	)
}

func mintFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: timeout}),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&MintFungibleTokenAttributes{
			Bearer:                           bearer,
			Type:                             parentTypeId,
			Value:                            fungibleTokenValue,
			TokenCreationPredicateSignatures: tokenCreationPredicateSignatures,
		}),
	)
}

func transferFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: timeout}),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&TransferFungibleTokenAttributes{
			NewBearer:                    newBearer,
			Type:                         parentTypeId,
			Value:                        transferValue,
			Nonce:                        nonce,
			Backlink:                     backlink,
			InvariantPredicateSignatures: invariantPredicateSignatures,
		}),
	)
}

func splitFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: timeout}),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&SplitFungibleTokenAttributes{
			Type:                         parentTypeId,
			NewBearer:                    newBearer,
			TargetValue:                  transferValue,
			RemainingValue:               remainingValue,
			Nonce:                        nonce,
			Backlink:                     backlink,
			InvariantPredicateSignatures: invariantPredicateSignatures,
		}),
	)
}

func burnFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: timeout}),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&BurnFungibleTokenAttributes{
			Type:                         burnType,
			Value:                        burnValue,
			Nonce:                        nonce,
			Backlink:                     backlink,
			InvariantPredicateSignatures: invariantPredicateSignatures,
		}),
	)
}

func createNonFungibleTokenTypeTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: timeout}),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                             symbol,
			ParentTypeId:                       parentTypeId,
			SubTypeCreationPredicate:           subTypeCreationPredicate,
			TokenCreationPredicate:             tokenCreationPredicate,
			InvariantPredicate:                 invariantPredicate,
			DataUpdatePredicate:                dataUpdatePredicate,
			SubTypeCreationPredicateSignatures: subTypeCreationPredicateSignatures,
		}),
	)
}

func createMintNonFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: timeout}),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                           bearer,
			NftType:                          nftType,
			Uri:                              uri,
			Data:                             data,
			DataUpdatePredicate:              dataUpdatePredicate,
			TokenCreationPredicateSignatures: tokenCreationPredicateSignatures,
		}),
	)
}

func createTransferNonFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: timeout}),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                    newBearer,
			NftType:                      nftType,
			Nonce:                        nonce,
			Backlink:                     backlink,
			InvariantPredicateSignatures: invariantPredicateSignatures,
		}),
	)
}

func createUpdateNonFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithClientMetadata(&txsystem.ClientMetadata{Timeout: timeout}),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:                 updatedData,
			Backlink:             backlink,
			DataUpdateSignatures: dataUpdateSignatures,
		}),
	)
}
