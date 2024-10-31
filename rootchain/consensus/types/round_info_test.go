package types

import (
	gocrypto "crypto"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/stretchr/testify/require"
)

func TestRoundInfo_Bytes(t *testing.T) {
	x := &RoundInfo{
		RoundNumber:       4,
		Epoch:             1,
		Timestamp:         0x12345678,
		ParentRoundNumber: 3,
		CurrentRootHash:   make([]byte, 32),
	}
	serialized := make([]byte, 0, 4*8+32)
	serialized = append(serialized, util.Uint64ToBytes(x.RoundNumber)...)
	serialized = append(serialized, util.Uint64ToBytes(x.Epoch)...)
	serialized = append(serialized, util.Uint64ToBytes(x.Timestamp)...)
	serialized = append(serialized, util.Uint64ToBytes(x.ParentRoundNumber)...)
	serialized = append(serialized, x.CurrentRootHash...)
	buf := x.Bytes()
	require.True(t, reflect.DeepEqual(buf, serialized))
}

func TestRoundInfo_GetParentRound(t *testing.T) {
	var x *RoundInfo = nil
	require.EqualValues(t, 0, x.GetRound())
	x = &RoundInfo{ParentRoundNumber: 3}
	require.EqualValues(t, 3, x.GetParentRound())
}

func TestRoundInfo_GetRound(t *testing.T) {
	var x *RoundInfo = nil
	require.EqualValues(t, 0, x.GetRound())
	x = &RoundInfo{RoundNumber: 3}
	require.EqualValues(t, 3, x.GetRound())
}

func TestRoundInfo_Hash(t *testing.T) {
	x := &RoundInfo{
		RoundNumber:       4,
		Epoch:             1,
		Timestamp:         0x12345678,
		ParentRoundNumber: 3,
		CurrentRootHash:   make([]byte, 32),
	}
	require.NotNil(t, x.Hash(gocrypto.SHA256))
}

func TestRoundInfo_IsValid(t *testing.T) {
	// valid round info obj - each test creates copy of it to make single field invalid
	validRI := RoundInfo{
		ParentRoundNumber: 22,
		RoundNumber:       23,
		Epoch:             0,
		Timestamp:         1257894000,
		CurrentRootHash:   []byte{0, 1, 2, 5},
	}
	require.NoError(t, validRI.IsValid())

	t.Run("parent round number is unassigned", func(t *testing.T) {
		ri := validRI
		ri.ParentRoundNumber = 0
		require.ErrorIs(t, ri.IsValid(), errParentRoundUnassigned)
	})

	t.Run("round number is unassigned", func(t *testing.T) {
		ri := validRI
		ri.RoundNumber = 0
		require.ErrorIs(t, ri.IsValid(), errRoundNumberUnassigned)
	})

	t.Run("parent round must be strictly smaller than current round", func(t *testing.T) {
		ri := validRI
		ri.RoundNumber = ri.ParentRoundNumber
		require.EqualError(t, ri.IsValid(), `invalid round number 22 - must be greater than parent round 22`)

		ri.RoundNumber = ri.ParentRoundNumber - 1
		require.EqualError(t, ri.IsValid(), `invalid round number 21 - must be greater than parent round 22`)
	})

	t.Run("root hash is empty", func(t *testing.T) {
		ri := validRI
		ri.CurrentRootHash = nil
		require.ErrorIs(t, ri.IsValid(), errRootHashUnassigned)

		ri.CurrentRootHash = []byte{}
		require.ErrorIs(t, ri.IsValid(), errRootHashUnassigned)
	})

	t.Run("timestamp unassigned", func(t *testing.T) {
		ri := validRI
		ri.Timestamp = 0
		require.ErrorIs(t, ri.IsValid(), errRoundCreationTimeNotSet)
	})
}
