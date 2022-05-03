package transaction

import (
	"testing"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/verifiable_data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapper_RegisterData(t *testing.T) {
	reg := &RegisterData{}
	tx := newPBTransactionOrder(test.RandomBytes(32), []byte{0}, 500, reg)
	genericTx, err := NewVerifiableDataTx(tx)
	require.NoError(t, err)

	switch w := genericTx.(type) {
	case verifiable_data.RegisterTx:
		assert.Equal(t, tx.Timeout, w.Timeout())
		id := w.UnitID().Bytes32()
		assert.Equal(t, tx.UnitId, id[:])
	default:
		require.Fail(t, "Transaction type conversion failed")
	}
}
