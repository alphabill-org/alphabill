package tokens

import (
	"bytes"
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	systemID                          = []byte{0, 0, 4, 2}
	unitID                            = []byte{1}
	ownerProof                        = []byte{2}
	symbol                            = "TEST"
	parentTypeId                      = []byte{3}
	subTypeCreationPredicate          = []byte{4}
	tokenCreationPredicate            = []byte{5}
	invariantPredicate                = []byte{6}
	dataUpdatePredicate               = []byte{7}
	subTypeCreationPredicateSignature = []byte{8}
	timeout                           = uint64(100)
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

func createNonFungibleTokenTypeTxOrder(t *testing.T, systemIdentifier []byte) *txsystem.Transaction {
	tx := &txsystem.Transaction{
		SystemId:              systemIdentifier,
		UnitId:                unitID,
		Timeout:               timeout,
		TransactionAttributes: &anypb.Any{},
		OwnerProof:            ownerProof,
	}
	attr := &CreateNonFungibleTokenTypeAttributes{
		Symbol:                            symbol,
		ParentTypeId:                      parentTypeId,
		SubTypeCreationPredicate:          subTypeCreationPredicate,
		TokenCreationPredicate:            tokenCreationPredicate,
		InvariantPredicate:                invariantPredicate,
		DataUpdatePredicate:               dataUpdatePredicate,
		SubTypeCreationPredicateSignature: subTypeCreationPredicateSignature,
	}
	// #nosec G104
	require.NoError(t, tx.TransactionAttributes.MarshalFrom(attr))
	return tx
}
