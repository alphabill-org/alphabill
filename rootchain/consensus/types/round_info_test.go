package types

import (
	gocrypto "crypto"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundInfo_MarshalCBOR(t *testing.T) {
	x := &RoundInfo{
		RoundNumber:       4,
		Epoch:             1,
		Timestamp:         0x12345678,
		ParentRoundNumber: 3,
		CurrentRootHash:   make([]byte, 32),
	}
	bs, err := x.MarshalCBOR()
	require.NoError(t, err)
	require.NotNil(t, bs)

	y := new(RoundInfo)
	err = y.UnmarshalCBOR(bs)
	require.NoError(t, err)
	require.Equal(t, x, y)
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
	h, err := x.Hash(gocrypto.SHA256)
	require.NoError(t, err)
	require.NotNil(t, h)
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

	t.Run("timestamp unassigned", func(t *testing.T) {
		ri := validRI
		ri.Timestamp = 0
		require.ErrorIs(t, ri.IsValid(), errRoundCreationTimeNotSet)
	})
}
