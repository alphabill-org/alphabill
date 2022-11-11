package tokens

import (
	"bytes"
	gocrypto "crypto"
	"testing"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

var (
	systemID                                 = []byte{0, 0, 4, 2}
	unitID                                   = []byte{1}
	ownerProof                               = []byte{2}
	symbol                                   = "TEST"
	parentTypeId                             = []byte{3}
	subTypeCreationPredicate                 = []byte{4}
	tokenCreationPredicate                   = []byte{5}
	invariantPredicate                       = []byte{6}
	dataUpdatePredicate                      = []byte{7}
	subTypeCreationPredicateSignature        = []byte{8}
	timeout                                  = uint64(100)
	bearer                                   = []byte{10}
	nftType                                  = []byte{11}
	data                                     = []byte{12}
	updatedData                              = []byte{0, 12}
	uri                                      = "https://alphabill.org"
	tokenCreationPredicateSignature          = []byte{14}
	newBearer                                = []byte{15}
	nonce                                    = []byte{16}
	backlink                                 = []byte{17}
	invariantPredicateSignature              = []byte{18}
	dataUpdateSignature                      = []byte{19}
	fungibleTokenDecimalPlaces        uint32 = 8
	fungibleTokenValue                uint64 = 100_000_002
	transferValue                     uint64 = 1000
	burnValue                         uint64 = 2000
	burnType                                 = []byte{20}
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
	b.Write([]byte(symbol))
	b.Write(parentTypeId)
	b.Write(subTypeCreationPredicate)
	b.Write(tokenCreationPredicate)
	b.Write(invariantPredicate)
	b.Write(dataUpdatePredicate)
	b.Write(subTypeCreationPredicateSignature)
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
	b.Write(bearer)
	b.Write(nftType)
	b.Write([]byte(uri))
	b.Write(data)
	b.Write(dataUpdatePredicate)
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
	b.Write(newBearer)
	b.Write(nonce)
	b.Write(backlink)
	b.Write(invariantPredicateSignature)
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
	b.Write(newBearer)
	b.Write(nonce)
	b.Write(backlink)
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
	b.Write(updatedData)
	b.Write(backlink)
	b.Write(dataUpdateSignature)
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
	b.Write([]byte(symbol))
	b.Write(parentTypeId)
	b.Write(util.Uint32ToBytes(fungibleTokenDecimalPlaces))
	b.Write(subTypeCreationPredicate)
	b.Write(tokenCreationPredicate)
	b.Write(invariantPredicate)
	b.Write(subTypeCreationPredicateSignature)
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
	b.Write(bearer)
	b.Write(parentTypeId)
	b.Write(util.Uint64ToBytes(fungibleTokenValue))
	b.Write(tokenCreationPredicateSignature)
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
	b.Write(newBearer)
	b.Write(util.Uint64ToBytes(transferValue))
	b.Write(nonce)
	b.Write(backlink)
	b.Write(invariantPredicateSignature)
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
	b.Write(newBearer)
	b.Write(util.Uint64ToBytes(transferValue))
	b.Write(nonce)
	b.Write(backlink)
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
	b.Write(newBearer)
	b.Write(util.Uint64ToBytes(transferValue))
	b.Write(nonce)
	b.Write(backlink)
	b.Write(invariantPredicateSignature)
	require.Equal(t, b.Sum(nil), hash)
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
	b.Write(newBearer)
	b.Write(util.Uint64ToBytes(transferValue))
	b.Write(nonce)
	b.Write(backlink)
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
	b.Write(burnType)
	b.Write(util.Uint64ToBytes(burnValue))
	b.Write(nonce)
	b.Write(backlink)
	b.Write(invariantPredicateSignature)
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
		testtransaction.WithTimeout(timeout),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&CreateFungibleTokenTypeAttributes{
			Symbol:                            symbol,
			ParentTypeId:                      parentTypeId,
			DecimalPlaces:                     fungibleTokenDecimalPlaces,
			SubTypeCreationPredicate:          subTypeCreationPredicate,
			TokenCreationPredicate:            tokenCreationPredicate,
			InvariantPredicate:                invariantPredicate,
			SubTypeCreationPredicateSignature: subTypeCreationPredicateSignature,
		}),
	)
}

func mintFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithTimeout(timeout),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&MintFungibleTokenAttributes{
			Bearer:                          bearer,
			Type:                            parentTypeId,
			Value:                           fungibleTokenValue,
			TokenCreationPredicateSignature: tokenCreationPredicateSignature,
		}),
	)
}

func transferFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithTimeout(timeout),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&TransferFungibleTokenAttributes{
			NewBearer:                   newBearer,
			Value:                       transferValue,
			Nonce:                       nonce,
			Backlink:                    backlink,
			InvariantPredicateSignature: invariantPredicateSignature,
		}),
	)
}

func splitFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithTimeout(timeout),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&SplitFungibleTokenAttributes{
			NewBearer:                   newBearer,
			TargetValue:                 transferValue,
			Nonce:                       nonce,
			Backlink:                    backlink,
			InvariantPredicateSignature: invariantPredicateSignature,
		}),
	)
}

func burnFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithTimeout(timeout),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&BurnFungibleTokenAttributes{
			Type:                        burnType,
			Value:                       burnValue,
			Nonce:                       nonce,
			Backlink:                    backlink,
			InvariantPredicateSignature: invariantPredicateSignature,
		}),
	)
}

func createNonFungibleTokenTypeTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithTimeout(timeout),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&CreateNonFungibleTokenTypeAttributes{
			Symbol:                            symbol,
			ParentTypeId:                      parentTypeId,
			SubTypeCreationPredicate:          subTypeCreationPredicate,
			TokenCreationPredicate:            tokenCreationPredicate,
			InvariantPredicate:                invariantPredicate,
			DataUpdatePredicate:               dataUpdatePredicate,
			SubTypeCreationPredicateSignature: subTypeCreationPredicateSignature,
		}),
	)
}

func createMintNonFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithTimeout(timeout),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&MintNonFungibleTokenAttributes{
			Bearer:                          bearer,
			NftType:                         nftType,
			Uri:                             uri,
			Data:                            data,
			DataUpdatePredicate:             dataUpdatePredicate,
			TokenCreationPredicateSignature: tokenCreationPredicateSignature,
		}),
	)
}

func createTransferNonFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithTimeout(timeout),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&TransferNonFungibleTokenAttributes{
			NewBearer:                   newBearer,
			Nonce:                       nonce,
			Backlink:                    backlink,
			InvariantPredicateSignature: invariantPredicateSignature,
		}),
	)
}

func createUpdateNonFungibleTokenTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	return testtransaction.NewTransaction(t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithUnitId(unitID),
		testtransaction.WithTimeout(timeout),
		testtransaction.WithOwnerProof(ownerProof),
		testtransaction.WithAttributes(&UpdateNonFungibleTokenAttributes{
			Data:                updatedData,
			Backlink:            backlink,
			DataUpdateSignature: dataUpdateSignature,
		}),
	)
}
