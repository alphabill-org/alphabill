package txsystem

import (
	"crypto"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"github.com/stretchr/testify/require"
)

func TestTransaction_Hash(t *testing.T) {
	tx := &Transaction{
		TransactionAttributes: new(anypb.Any),
		UnitId:                test.RandomBytes(3),
		Timeout:               0,
		OwnerProof:            test.RandomBytes(3),
	}

	result, err := tx.Hash(crypto.SHA256)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 32, len(result))
}
