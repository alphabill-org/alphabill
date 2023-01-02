package txsystem

import (
	"bytes"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestTransaction_BytesIsCalculatedCorrectly(t *testing.T) {
	tx := &Transaction{
		SystemId:   test.RandomBytes(32),
		UnitId:     test.RandomBytes(32),
		Timeout:    1,
		OwnerProof: test.RandomBytes(32),
		FeeProof:   test.RandomBytes(32),
		ClientMetadata: &ClientMetadata{
			Timeout:           2,
			MaxFee:            3,
			FeeCreditRecordId: 4,
		},
		ServerMetadata: &ServerMetadata{
			Fee: 5,
		},
	}
	var b bytes.Buffer
	b.Write(tx.SystemId)
	b.Write(tx.UnitId)
	b.Write(util.Uint64ToBytes(tx.Timeout))
	b.Write(tx.OwnerProof)
	b.Write(tx.FeeProof)
	cm := tx.ClientMetadata
	b.Write(util.Uint64ToBytes(cm.Timeout))
	b.Write(util.Uint64ToBytes(cm.MaxFee))
	b.Write(util.Uint64ToBytes(cm.FeeCreditRecordId))
	sm := tx.ServerMetadata
	b.Write(util.Uint64ToBytes(sm.Fee))
	expectedBytes := b.Bytes()

	require.Equal(t, expectedBytes, tx.Bytes())
}
